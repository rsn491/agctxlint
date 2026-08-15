// Package lint applies ctxlint's rules to agent instruction files.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rsn491/ctxlint/internal/discover"
	"github.com/rsn491/ctxlint/internal/parse"
	"github.com/rsn491/ctxlint/internal/tokens"
	"gopkg.in/yaml.v3"
)

// Severity marks how a finding affects the exit code.
type Severity string

const (
	// SeverityError fails the run.
	SeverityError Severity = "error"
	// SeverityWarning is reported but passes, unless Config.Strict is set.
	SeverityWarning Severity = "warning"
)

// Rule ids, usable with Config.Disabled.
const (
	RuleFrontmatterMissing      = "frontmatter.missing"
	RuleFrontmatterNotFirst     = "frontmatter.not-first"
	RuleFrontmatterUnterminated = "frontmatter.unterminated"
	RuleFrontmatterInvalid      = "frontmatter.invalid"
	RuleFrontmatterUnknownKey   = "frontmatter.unknown-key"
	RuleNameRequired            = "name.required"
	RuleNameFormat              = "name.format"
	RuleNameLength              = "name.length"
	RuleNameDirMismatch         = "name.dir-mismatch"
	RuleDescriptionRequired     = "description.required"
	RuleDescriptionLength       = "description.length"
	RuleAllowedToolsType        = "allowed-tools.type"
	RuleMetadataType            = "metadata.type"
	RuleTokensContent           = "tokens.content"
	RuleTokensName              = "tokens.name"
	RuleTokensDescription       = "tokens.description"
	RuleFileReferenceMissing    = "file-reference.missing"
)

// Rules lists every rule id ctxlint can emit, in report order.
var Rules = []string{
	RuleFrontmatterMissing,
	RuleFrontmatterNotFirst,
	RuleFrontmatterUnterminated,
	RuleFrontmatterInvalid,
	RuleFrontmatterUnknownKey,
	RuleNameRequired,
	RuleNameFormat,
	RuleNameLength,
	RuleNameDirMismatch,
	RuleDescriptionRequired,
	RuleDescriptionLength,
	RuleAllowedToolsType,
	RuleMetadataType,
	RuleTokensContent,
	RuleTokensName,
	RuleTokensDescription,
	RuleFileReferenceMissing,
}

// Limits from the Anthropic skill spec, in characters.
const (
	MaxNameChars        = 64
	MaxDescriptionChars = 1024
)

// knownKeys are the front-matter keys the skill spec defines, plus the Claude
// Code extensions ctxlint additionally supports. Anything else is reported as
// an unknown key.
var knownKeys = map[string]bool{
	"name":                     true,
	"description":              true,
	"license":                  true,
	"compatibility":            true,
	"metadata":                 true,
	"allowed-tools":            true,
	"disable-model-invocation": true,
	"argument-hint":            true,
}

// nameRE is the spec's naming rule: lowercase alphanumerics in hyphen-separated
// segments.
var nameRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// linkRE matches inline markdown links and images: [text](target) or
// ![alt](target). It does not handle reference-style links ([text][ref]).
var linkRE = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)

// uriSchemeRE detects an absolute URI (https://, mailto:, tel:, and the like)
// so those targets are left for a browser rather than checked as files.
var uriSchemeRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// Config holds the thresholds and switches that shape a run.
type Config struct {
	// MaxAgentsTokens and MaxSkillTokens are the body token budgets, one per
	// file kind. Zero disables the check for that kind.
	MaxAgentsTokens int
	MaxSkillTokens  int
	// MaxSkillNameTokens and MaxSkillDescriptionTokens apply to skills only.
	// Zero disables the check.
	MaxSkillNameTokens        int
	MaxSkillDescriptionTokens int
	// Disabled holds rule ids to skip.
	Disabled []string
	// Strict reports warnings as errors.
	Strict bool
}

// contentLimit returns the body budget for kind.
func (c Config) contentLimit(kind discover.Kind) int {
	if kind == discover.KindAgents {
		return c.MaxAgentsTokens
	}
	return c.MaxSkillTokens
}

// Finding is a single rule violation.
type Finding struct {
	File     string   `json:"file"`
	Line     int      `json:"line,omitempty"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// Counts records the measured token cost of a file's parts.
type Counts struct {
	Content     int `json:"content"`
	Name        int `json:"name,omitempty"`
	Description int `json:"description,omitempty"`
}

// Result is everything ctxlint learned about one file.
type Result struct {
	Path     string        `json:"path"`
	Kind     discover.Kind `json:"kind"`
	Tokens   Counts        `json:"tokens"`
	Findings []Finding     `json:"findings"`
}

// Errors returns the number of findings that fail the run.
func (r Result) Errors() int { return r.count(SeverityError) }

// Warnings returns the number of non-failing findings.
func (r Result) Warnings() int { return r.count(SeverityWarning) }

func (r Result) count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// Linter applies Config's rules using a token Counter.
type Linter struct {
	cfg      Config
	counter  tokens.Counter
	disabled map[string]bool
}

// New returns a Linter. A nil counter falls back to the heuristic estimator.
func New(cfg Config, counter tokens.Counter) *Linter {
	if counter == nil {
		counter = tokens.NewEstimator()
	}
	disabled := make(map[string]bool, len(cfg.Disabled))
	for _, rule := range cfg.Disabled {
		disabled[rule] = true
	}
	return &Linter{cfg: cfg, counter: counter, disabled: disabled}
}

// File reads and lints one target.
func (l *Linter) File(t discover.Target) (Result, error) {
	src, err := os.ReadFile(t.Path)
	if err != nil {
		return Result{}, fmt.Errorf("cannot read %s: %w", t.Path, err)
	}
	return l.Check(t, src), nil
}

// Check lints already-read file contents. Findings are ordered by rule so
// output does not depend on the order the checks happen to run in.
func (l *Linter) Check(t discover.Target, src []byte) Result {
	res := Result{Path: t.Path, Kind: t.Kind, Findings: []Finding{}}

	fm, body, err := parse.Split(src)
	res.Tokens.Content = l.counter.Count(body)

	add := func(rule string, sev Severity, line int, format string, args ...any) {
		if l.disabled[rule] {
			return
		}
		if sev == SeverityWarning && l.cfg.Strict {
			sev = SeverityError
		}
		res.Findings = append(res.Findings, Finding{
			File:     t.Path,
			Line:     line,
			Rule:     rule,
			Severity: sev,
			Message:  fmt.Sprintf(format, args...),
		})
	}

	if t.Kind == discover.KindSkill {
		l.checkSkill(t, fm, err, add, &res)
	} else if perr, ok := err.(*parse.Error); ok && perr.Kind == parse.KindUnterminated {
		// AGENTS.md front matter is optional and unvalidated, but an unclosed
		// fence means the whole file is being read as front matter.
		add(RuleFrontmatterUnterminated, SeverityError, perr.Line, "%s", perr.Msg)
	}

	l.checkFileReferences(t, fm, body, add)

	if limit := l.cfg.contentLimit(t.Kind); limit > 0 && res.Tokens.Content > limit {
		add(RuleTokensContent, SeverityError, 0,
			"content is %s tokens, over the %s token limit",
			humanize(res.Tokens.Content), humanize(limit))
	}

	sortFindings(res.Findings)
	return res
}

type addFunc func(rule string, sev Severity, line int, format string, args ...any)

func (l *Linter) checkSkill(t discover.Target, fm parse.Frontmatter, err error, add addFunc, res *Result) {
	if err != nil {
		perr, ok := err.(*parse.Error)
		if !ok {
			add(RuleFrontmatterInvalid, SeverityError, 0, "%s", err.Error())
			return
		}
		switch perr.Kind {
		case parse.KindNotFirst:
			add(RuleFrontmatterNotFirst, SeverityError, perr.Line, "%s", perr.Msg)
		case parse.KindUnterminated:
			add(RuleFrontmatterUnterminated, SeverityError, perr.Line, "%s", perr.Msg)
		default:
			add(RuleFrontmatterInvalid, SeverityError, perr.Line, "%s", perr.Msg)
		}
		return
	}
	if !fm.Present {
		add(RuleFrontmatterMissing, SeverityError, 1,
			"missing YAML front matter: a skill must open with a --- fenced block declaring name and description")
		return
	}

	for _, key := range fm.Keys() {
		if !knownKeys[key] {
			add(RuleFrontmatterUnknownKey, SeverityWarning, fm.Line(key),
				"unknown front-matter key %q", key)
		}
	}

	l.checkName(t, fm, add, res)
	l.checkDescription(fm, add, res)
	l.checkAllowedTools(fm, add)
	l.checkMetadata(fm, add)
}

func (l *Linter) checkName(t discover.Target, fm parse.Frontmatter, add addFunc, res *Result) {
	name, ok := fm.String("name")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		add(RuleNameRequired, SeverityError, fm.Line("name"),
			"front matter must declare a non-empty string name")
		return
	}
	res.Tokens.Name = l.counter.Count(name)
	line := fm.Line("name")

	if !nameRE.MatchString(name) {
		add(RuleNameFormat, SeverityError, line,
			"name %q must be lowercase letters, digits and single hyphens (for example my-skill)", name)
	}
	if n := len([]rune(name)); n > MaxNameChars {
		add(RuleNameLength, SeverityError, line,
			"name is %d characters, over the %d character limit", n, MaxNameChars)
	}
	if dir := skillDir(t.Path); dir != "" && dir != name {
		add(RuleNameDirMismatch, SeverityWarning, line,
			"name %q does not match its directory %q", name, dir)
	}
	if l.cfg.MaxSkillNameTokens > 0 && res.Tokens.Name > l.cfg.MaxSkillNameTokens {
		add(RuleTokensName, SeverityError, line,
			"name is %s tokens, over the %s token limit",
			humanize(res.Tokens.Name), humanize(l.cfg.MaxSkillNameTokens))
	}
}

func (l *Linter) checkDescription(fm parse.Frontmatter, add addFunc, res *Result) {
	desc, ok := fm.String("description")
	desc = strings.TrimSpace(desc)
	if !ok || desc == "" {
		add(RuleDescriptionRequired, SeverityError, fm.Line("description"),
			"front matter must declare a non-empty string description saying when to use the skill")
		return
	}
	res.Tokens.Description = l.counter.Count(desc)
	line := fm.Line("description")

	if n := len([]rune(desc)); n > MaxDescriptionChars {
		add(RuleDescriptionLength, SeverityError, line,
			"description is %s characters, over the %s character limit",
			humanize(n), humanize(MaxDescriptionChars))
	}
	if l.cfg.MaxSkillDescriptionTokens > 0 && res.Tokens.Description > l.cfg.MaxSkillDescriptionTokens {
		add(RuleTokensDescription, SeverityError, line,
			"description is %s tokens, over the %s token limit",
			humanize(res.Tokens.Description), humanize(l.cfg.MaxSkillDescriptionTokens))
	}
}

func (l *Linter) checkAllowedTools(fm parse.Frontmatter, add addFunc) {
	if !fm.Has("allowed-tools") {
		return
	}
	if _, ok := fm.StringSlice("allowed-tools"); !ok {
		add(RuleAllowedToolsType, SeverityError, fm.Line("allowed-tools"),
			"allowed-tools must be a list of tool names or a comma-separated string")
	}
}

func (l *Linter) checkMetadata(fm parse.Frontmatter, add addFunc) {
	node := fm.Node("metadata")
	if node == nil {
		return
	}
	if node.Kind != yaml.MappingNode {
		add(RuleMetadataType, SeverityError, fm.Line("metadata"),
			"metadata must be a mapping of keys to values")
	}
}

// checkFileReferences flags markdown links whose target looks like a local
// file but does not resolve to one, relative to the linted file's directory.
// It applies to both AGENTS.md and SKILL.md, since a dangling reference breaks
// the file's contract with the runtime regardless of kind.
func (l *Linter) checkFileReferences(t discover.Target, fm parse.Frontmatter, body string, add addFunc) {
	dir := filepath.Dir(t.Path)
	offset := 0
	if fm.Present {
		offset = fm.EndLine
	}

	inFence := false
	for i, line := range strings.Split(body, "\n") {
		fence := strings.TrimSpace(line)
		if strings.HasPrefix(fence, "```") || strings.HasPrefix(fence, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, m := range linkRE.FindAllStringSubmatch(line, -1) {
			target := linkTargetPath(m[1])
			if target == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, target)); err != nil {
				add(RuleFileReferenceMissing, SeverityError, offset+i+1,
					"referenced file %q does not exist", target)
			}
		}
	}
}

// linkTargetPath extracts the file path a markdown link points at, or ""
// when the link is not a checkable local file reference: an absolute URL, an
// in-page anchor, an absolute path, or a templated placeholder.
func linkTargetPath(raw string) string {
	target := strings.TrimSpace(raw)
	if strings.HasPrefix(target, "<") {
		if end := strings.Index(target, ">"); end != -1 {
			target = target[1:end]
		}
	}
	if sp := strings.IndexAny(target, " \t"); sp != -1 {
		target = target[:sp]
	}
	if target == "" || strings.HasPrefix(target, "#") {
		return ""
	}
	if uriSchemeRE.MatchString(target) || strings.HasPrefix(target, "//") {
		return ""
	}
	if frag := strings.IndexAny(target, "#?"); frag != -1 {
		target = target[:frag]
	}
	if target == "" || filepath.IsAbs(target) || strings.ContainsAny(target, "{}$*") {
		return ""
	}
	return target
}

// skillDir returns the name of the directory holding the skill file, or "" when
// there is no meaningful directory to match the name against: a loose skill file
// in the working directory is named after the project, not the skill.
func skillDir(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	if cwd, err := os.Getwd(); err == nil && dir == cwd {
		return ""
	}
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

// ruleOrder indexes Rules for stable sorting.
var ruleOrder = func() map[string]int {
	order := make(map[string]int, len(Rules))
	for i, rule := range Rules {
		order[rule] = i
	}
	return order
}()

func sortFindings(findings []Finding) {
	// Insertion sort: findings per file are few, and this keeps the relative
	// order of same-rule findings (for example several unknown keys).
	for i := 1; i < len(findings); i++ {
		for j := i; j > 0 && ruleOrder[findings[j].Rule] < ruleOrder[findings[j-1].Rule]; j-- {
			findings[j], findings[j-1] = findings[j-1], findings[j]
		}
	}
}

// humanize renders n with thousands separators, so "6,142 tokens" reads at a
// glance in reports.
func humanize(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return b.String()
}
