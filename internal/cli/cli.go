// Package cli wires flags, discovery, linting and reporting together.
package cli

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/rsn491/ctxlint/internal/discover"
	"github.com/rsn491/ctxlint/internal/lint"
	"github.com/rsn491/ctxlint/internal/report"
)

// Exit codes.
const (
	// ExitOK means no errors were found; warnings alone still exit OK.
	ExitOK = 0
	// ExitFindings means at least one error-severity finding was reported.
	ExitFindings = 1
	// ExitUsage means the run could not happen: bad flags or unreadable files.
	ExitUsage = 2
)

// Version is the reported build version, overridable at link time with
// -ldflags "-X github.com/rsn491/ctxlint/internal/cli.Version=v1.2.3".
var Version = "dev"

// Default thresholds. Zero disables a check, so defaults are the only place
// these numbers live.
const (
	// defaultMaxAgentsTokens follows Anthropic's guidance to keep AGENTS.md
	// to roughly 200 lines or less.
	defaultMaxAgentsTokens           = 2000
	defaultMaxSkillTokens            = 5000
	defaultMaxSkillNameTokens        = 16
	defaultMaxSkillDescriptionTokens = 100
)

// repeatable collects a flag that may be given more than once.
type repeatable []string

func (r *repeatable) String() string { return strings.Join(*r, ",") }

func (r *repeatable) Set(v string) error {
	if v == "" {
		return fmt.Errorf("value must not be empty")
	}
	*r = append(*r, v)
	return nil
}

// App holds one invocation's parsed flags and I/O streams.
type App struct {
	stdout, stderr io.Writer
	fs             *flag.FlagSet

	maxAgents, maxSkillBody, maxSkillName, maxSkillDescription int
	format                                                     string
	strict, quiet, showVersion, listRules                      bool
	excludes, disabled                                         repeatable
	paths                                                      []string
}

// Run executes ctxlint and returns the process exit code. Findings go to stdout;
// usage and I/O problems go to stderr.
func Run(args []string, stdout, stderr io.Writer) int {
	a := &App{stdout: stdout, stderr: stderr}
	return a.run(args)
}

func (a *App) run(args []string) int {
	code, done := a.parseFlags(args)
	if done {
		return code
	}

	if err := a.validate(); err != nil {
		fmt.Fprintf(a.stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}

	return a.execute()
}

// parseFlags parses args into a's fields. done is true when the run is
// already over, either from an error or from a flag like -version that exits
// before any linting happens.
func (a *App) parseFlags(args []string) (code int, done bool) {
	fs := flag.NewFlagSet("ctxlint", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	fs.Usage = func() { usage(a.stderr, fs) }
	a.fs = fs

	maxAgents := fs.Int("max-agents-tokens", defaultMaxAgentsTokens, "token budget for AGENTS.md content (0 disables)")
	maxSkillBody := fs.Int("max-skill-tokens", defaultMaxSkillTokens, "token budget for SKILL.md content (0 disables)")
	maxSkillName := fs.Int("max-skill-name-tokens", defaultMaxSkillNameTokens, "token budget for a skill's name (0 disables)")
	maxSkillDescription := fs.Int("max-skill-description-tokens", defaultMaxSkillDescriptionTokens, "token budget for a skill's description (0 disables)")
	format := fs.String("format", "text", "output format: text or json")
	strict := fs.Bool("strict", false, "treat warnings as errors")
	quiet := fs.Bool("quiet", false, "report errors only")
	showVersion := fs.Bool("version", false, "print the version and exit")
	listRules := fs.Bool("list-rules", false, "print every rule id and exit")
	fs.Var(&a.excludes, "exclude", "glob of paths to skip; repeatable")
	fs.Var(&a.disabled, "disable", "rule id to skip; repeatable")

	if err := fs.Parse(args); err != nil {
		// ContinueOnError already printed the problem and the usage text.
		if err == flag.ErrHelp {
			return ExitOK, true
		}
		return ExitUsage, true
	}

	a.maxAgents, a.maxSkillBody = *maxAgents, *maxSkillBody
	a.maxSkillName, a.maxSkillDescription = *maxSkillName, *maxSkillDescription
	a.format, a.strict, a.quiet = *format, *strict, *quiet
	a.showVersion, a.listRules = *showVersion, *listRules

	if a.showVersion {
		fmt.Fprintf(a.stdout, "ctxlint %s\n", Version)
		return ExitOK, true
	}
	if a.listRules {
		for _, rule := range lint.Rules {
			fmt.Fprintln(a.stdout, rule)
		}
		return ExitOK, true
	}

	a.paths = fs.Args()
	if len(a.paths) == 0 {
		a.paths = []string{"."}
	}
	return ExitOK, false
}

// validate checks the parsed flags for problems that fs.Parse itself cannot
// catch: unknown -format values, typoed rule ids, and negative budgets.
func (a *App) validate() error {
	if a.format != "text" && a.format != "json" {
		return fmt.Errorf("unknown -format %q: want text or json", a.format)
	}
	if err := a.checkRuleNames(); err != nil {
		return err
	}
	return a.checkNonNegative()
}

// checkRuleNames rejects typos in -disable rather than silently doing nothing.
func (a *App) checkRuleNames() error {
	known := make(map[string]bool, len(lint.Rules))
	for _, rule := range lint.Rules {
		known[rule] = true
	}
	var unknown []string
	for _, rule := range a.disabled {
		if !known[rule] {
			unknown = append(unknown, rule)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("unknown rule %s in -disable: run -list-rules to see them all",
		strings.Join(quoteAll(unknown), ", "))
}

// checkNonNegative guards against a negative budget, which would otherwise read
// as "disabled" and quietly skip the check the user asked for.
func (a *App) checkNonNegative() error {
	var bad []string
	a.fs.Visit(func(f *flag.Flag) {
		g, ok := f.Value.(flag.Getter)
		if !ok {
			return
		}
		if n, isInt := g.Get().(int); isInt && n < 0 {
			bad = append(bad, "-"+f.Name)
		}
	})
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("%s must be zero or more (0 disables the check)", strings.Join(bad, ", "))
}

// execute discovers, lints and reports on the configured paths.
func (a *App) execute() int {
	d, err := discover.New(a.excludes)
	if err != nil {
		fmt.Fprintf(a.stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}
	targets, err := d.Find(a.paths)
	if err != nil {
		fmt.Fprintf(a.stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}

	linter := lint.New(lint.Config{
		MaxAgentsTokens:           a.maxAgents,
		MaxSkillTokens:            a.maxSkillBody,
		MaxSkillNameTokens:        a.maxSkillName,
		MaxSkillDescriptionTokens: a.maxSkillDescription,
		Disabled:                  a.disabled,
		Strict:                    a.strict,
	}, nil)

	results := make([]lint.Result, 0, len(targets))
	for _, t := range targets {
		res, err := linter.File(t)
		if err != nil {
			fmt.Fprintf(a.stderr, "ctxlint: %v\n", err)
			return ExitUsage
		}
		results = append(results, res)
	}

	rep := report.NewReporter(a.stdout, a.quiet)
	if a.format == "json" {
		err = rep.JSON(results)
	} else {
		err = rep.Text(results)
	}
	if err != nil {
		fmt.Fprintf(a.stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}

	if report.Summarize(results).Errors > 0 {
		return ExitFindings
	}
	return ExitOK
}

func quoteAll(values []string) []string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return quoted
}

func usage(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprint(w, `ctxlint lints agent instruction files: AGENTS.md and SKILL.md.

Usage:
  ctxlint [flags] [path...]

Paths may be files or directories; directories are walked recursively for
AGENTS.md and SKILL.md. With no path given, the current directory is used.

For skills, YAML front matter is validated against the skill spec. For both
kinds, token budgets are enforced on the content, and on a skill's name and
description.

Exit codes: 0 clean (warnings still exit 0), 1 errors found, 2 bad usage.

Flags:
`)
	fs.PrintDefaults()
}
