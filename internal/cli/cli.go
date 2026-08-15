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
	defaultMaxAgentsTokens           = 5000
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

// Run executes ctxlint and returns the process exit code. Findings go to stdout;
// usage and I/O problems go to stderr.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ctxlint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr, fs) }

	var (
		maxAgents           = fs.Int("max-agents-tokens", defaultMaxAgentsTokens, "token budget for AGENTS.md content (0 disables)")
		maxSkillBody        = fs.Int("max-skill-tokens", defaultMaxSkillTokens, "token budget for SKILL.md content (0 disables)")
		maxSkillName        = fs.Int("max-skill-name-tokens", defaultMaxSkillNameTokens, "token budget for a skill's name (0 disables)")
		maxSkillDescription = fs.Int("max-skill-description-tokens", defaultMaxSkillDescriptionTokens, "token budget for a skill's description (0 disables)")
		format              = fs.String("format", "text", "output format: text or json")
		strict              = fs.Bool("strict", false, "treat warnings as errors")
		quiet               = fs.Bool("quiet", false, "report errors only")
		showVersion         = fs.Bool("version", false, "print the version and exit")
		listRules           = fs.Bool("list-rules", false, "print every rule id and exit")
		excludes            repeatable
		disabled            repeatable
	)
	fs.Var(&excludes, "exclude", "glob of paths to skip; repeatable")
	fs.Var(&disabled, "disable", "rule id to skip; repeatable")

	if err := fs.Parse(args); err != nil {
		// ContinueOnError already printed the problem and the usage text.
		if err == flag.ErrHelp {
			return ExitOK
		}
		return ExitUsage
	}

	if *showVersion {
		fmt.Fprintf(stdout, "ctxlint %s\n", Version)
		return ExitOK
	}
	if *listRules {
		for _, rule := range lint.Rules {
			fmt.Fprintln(stdout, rule)
		}
		return ExitOK
	}

	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "ctxlint: unknown -format %q: want text or json\n", *format)
		return ExitUsage
	}
	if err := checkRuleNames(disabled); err != nil {
		fmt.Fprintf(stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}
	if err := checkNonNegative(fs); err != nil {
		fmt.Fprintf(stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}

	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	targets, err := discover.Find(paths, excludes)
	if err != nil {
		fmt.Fprintf(stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}

	linter := lint.New(lint.Config{
		MaxAgentsTokens:           *maxAgents,
		MaxSkillTokens:            *maxSkillBody,
		MaxSkillNameTokens:        *maxSkillName,
		MaxSkillDescriptionTokens: *maxSkillDescription,
		Disabled:                  disabled,
		Strict:                    *strict,
	}, nil)

	results := make([]lint.Result, 0, len(targets))
	for _, t := range targets {
		res, err := linter.File(t)
		if err != nil {
			fmt.Fprintf(stderr, "ctxlint: %v\n", err)
			return ExitUsage
		}
		results = append(results, res)
	}

	if *format == "json" {
		err = report.JSON(stdout, results, *quiet)
	} else {
		err = report.Text(stdout, results, *quiet)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ctxlint: %v\n", err)
		return ExitUsage
	}

	if report.Summarize(results).Errors > 0 {
		return ExitFindings
	}
	return ExitOK
}

// checkRuleNames rejects typos in -disable rather than silently doing nothing.
func checkRuleNames(rules []string) error {
	known := make(map[string]bool, len(lint.Rules))
	for _, rule := range lint.Rules {
		known[rule] = true
	}
	var unknown []string
	for _, rule := range rules {
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
func checkNonNegative(fs *flag.FlagSet) error {
	var bad []string
	fs.Visit(func(f *flag.Flag) {
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
