package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsn491/ctxlint/internal/lint"
	"github.com/rsn491/ctxlint/internal/report"
)

// fixture resolves a path inside the repository's testdata tree, which lives two
// directories above this package.
func fixture(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...)
}

// run executes the CLI in-process and returns its exit code and streams.
func run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCleanTreeExitsZero(t *testing.T) {
	code, stdout, stderr := run(fixture("clean"))
	if code != ExitOK {
		t.Errorf("exit = %d, want %d (stdout: %s stderr: %s)", code, ExitOK, stdout, stderr)
	}
	if !strings.Contains(stdout, "2 files checked, 0 errors, 0 warnings") {
		t.Errorf("stdout = %q, want a clean summary for both fixture files", stdout)
	}
}

func TestBrokenTreeExitsOne(t *testing.T) {
	code, stdout, _ := run(fixture("broken"))
	if code != ExitFindings {
		t.Errorf("exit = %d, want %d", code, ExitFindings)
	}

	for _, want := range []string{
		lint.RuleFrontmatterMissing,
		lint.RuleFrontmatterUnterminated,
		lint.RuleNameFormat,
		lint.RuleNameDirMismatch,
		lint.RuleFrontmatterUnknownKey,
		lint.RuleDescriptionLength,
		lint.RuleTokensDescription,
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not report %s:\n%s", want, stdout)
		}
	}

	if strings.Contains(stdout, "node_modules") {
		t.Errorf("stdout reports a node_modules file, want it pruned:\n%s", stdout)
	}
}

func TestTextOutputFormat(t *testing.T) {
	code, stdout, _ := run(fixture("broken", "bad-name", "SKILL.md"))
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d", code, ExitFindings)
	}

	var header, nameFormat string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasSuffix(line, "SKILL.md") {
			header = line
		}
		if strings.Contains(line, lint.RuleNameFormat) {
			nameFormat = line
		}
	}
	if header == "" {
		t.Fatalf("stdout has no SKILL.md file header:\n%s", stdout)
	}
	if nameFormat == "" {
		t.Fatalf("stdout has no %s line:\n%s", lint.RuleNameFormat, stdout)
	}
	// "  line: severity: rule: message", indented under the file header.
	trimmed := strings.TrimPrefix(nameFormat, "  ")
	parts := strings.SplitN(trimmed, ": ", 4)
	if len(parts) != 4 {
		t.Fatalf("finding line = %q, want four colon-separated fields", nameFormat)
	}
	if parts[0] != "2" {
		t.Errorf("line = %q, want %q", parts[0], "2")
	}
	if parts[1] != string(lint.SeverityError) {
		t.Errorf("severity = %q, want %q", parts[1], lint.SeverityError)
	}
	if parts[2] != lint.RuleNameFormat {
		t.Errorf("rule = %q, want %q", parts[2], lint.RuleNameFormat)
	}
}

func TestJSONOutput(t *testing.T) {
	code, stdout, _ := run("-format", "json", fixture("broken"))
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d", code, ExitFindings)
	}

	var got struct {
		Version int            `json:"version"`
		Files   []lint.Result  `json:"files"`
		Summary report.Summary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
	}
	if got.Version != report.SchemaVersion {
		t.Errorf("version = %d, want %d", got.Version, report.SchemaVersion)
	}
	if got.Summary.Files != len(got.Files) {
		t.Errorf("summary.files = %d, want %d", got.Summary.Files, len(got.Files))
	}
	if got.Summary.Errors == 0 {
		t.Error("summary.errors = 0, want the fixture errors counted")
	}

	// Files are sorted by path, and every finding carries its file back.
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
		if f.Kind == "" {
			t.Errorf("%s has no kind", f.Path)
		}
		for _, finding := range f.Findings {
			if finding.File != f.Path {
				t.Errorf("finding %s has file %q under result %q", finding.Rule, finding.File, f.Path)
			}
			if finding.Message == "" {
				t.Errorf("finding %s has no message", finding.Rule)
			}
		}
	}
	if !sorted(paths) {
		t.Errorf("paths = %v, want them sorted", paths)
	}
}

func TestJSONReportsTokenCounts(t *testing.T) {
	_, stdout, _ := run("-format", "json", fixture("clean", "skills", "well-formed", "SKILL.md"))

	var got struct {
		Files []lint.Result `json:"files"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout)
	}
	if len(got.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(got.Files))
	}
	tok := got.Files[0].Tokens
	if tok.Content <= 0 || tok.Name <= 0 || tok.Description <= 0 {
		t.Errorf("tokens = %+v, want content, name and description all counted", tok)
	}
}

func TestContentBudgetFlags(t *testing.T) {
	clean := fixture("clean")

	// Each kind's budget is independent.
	code, stdout, _ := run("-max-agents-tokens", "5", "-max-skill-tokens", "0", clean)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d:\n%s", code, ExitFindings, stdout)
	}
	if !strings.Contains(stdout, "AGENTS.md") || strings.Contains(stdout, "SKILL.md") {
		t.Errorf("stdout = %q, want only AGENTS.md over budget", stdout)
	}

	code, stdout, _ = run("-max-agents-tokens", "0", "-max-skill-tokens", "5", clean)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d:\n%s", code, ExitFindings, stdout)
	}
	if !strings.Contains(stdout, "SKILL.md") || strings.Contains(stdout, "AGENTS.md") {
		t.Errorf("stdout = %q, want only SKILL.md over budget", stdout)
	}

	if code, stdout, _ := run("-max-agents-tokens", "0", "-max-skill-tokens", "0", clean); code != ExitOK {
		t.Errorf("exit = %d with both budgets disabled, want %d:\n%s", code, ExitOK, stdout)
	}
}

func TestNameAndDescriptionBudgetFlags(t *testing.T) {
	skill := fixture("clean", "skills", "well-formed", "SKILL.md")

	code, stdout, _ := run("-max-skill-tokens", "0", "-max-skill-name-tokens", "1", skill)
	if code != ExitFindings || !strings.Contains(stdout, lint.RuleTokensName) {
		t.Errorf("exit = %d, stdout = %q; want %s reported", code, stdout, lint.RuleTokensName)
	}

	code, stdout, _ = run("-max-skill-tokens", "0", "-max-skill-description-tokens", "5", skill)
	if code != ExitFindings || !strings.Contains(stdout, lint.RuleTokensDescription) {
		t.Errorf("exit = %d, stdout = %q; want %s reported", code, stdout, lint.RuleTokensDescription)
	}

	// Defaults leave this fixture alone.
	if code, stdout, _ := run(skill); code != ExitOK {
		t.Errorf("exit = %d with default budgets, want %d:\n%s", code, ExitOK, stdout)
	}
}

func TestWarningsAloneExitZero(t *testing.T) {
	skill := fixture("broken", "bad-name", "SKILL.md")

	// name.format is the only error in this fixture; disabling it leaves warnings.
	code, stdout, _ := run("-disable", lint.RuleNameFormat, skill)
	if code != ExitOK {
		t.Errorf("exit = %d with warnings only, want %d:\n%s", code, ExitOK, stdout)
	}
	if !strings.Contains(stdout, lint.RuleNameDirMismatch) {
		t.Errorf("stdout = %q, want the warning still reported", stdout)
	}

	// Under -strict the same run fails.
	if code, _, _ := run("-strict", "-disable", lint.RuleNameFormat, skill); code != ExitFindings {
		t.Errorf("exit = %d under -strict, want %d", code, ExitFindings)
	}
}

func TestQuietSuppressesWarnings(t *testing.T) {
	skill := fixture("broken", "bad-name", "SKILL.md")

	_, stdout, _ := run("-quiet", skill)
	if strings.Contains(stdout, lint.RuleNameDirMismatch) {
		t.Errorf("stdout = %q, want warnings suppressed", stdout)
	}
	if !strings.Contains(stdout, lint.RuleNameFormat) {
		t.Errorf("stdout = %q, want errors kept", stdout)
	}
	// The summary still counts what was suppressed.
	if !strings.Contains(stdout, "warning") {
		t.Errorf("stdout = %q, want the summary to mention warnings", stdout)
	}

	_, stdout, _ = run("-quiet", "-format", "json", skill)
	if strings.Contains(stdout, lint.RuleNameDirMismatch) {
		t.Errorf("json = %q, want warnings suppressed", stdout)
	}
	var got struct {
		Summary report.Summary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Summary.Warnings == 0 {
		t.Error("summary.warnings = 0, want suppressed warnings still counted")
	}
}

func TestExcludePrunesPaths(t *testing.T) {
	code, stdout, _ := run("-exclude", "verbose-description", "-exclude", "no-frontmatter",
		"-exclude", "unterminated", "-exclude", "bad-name", fixture("broken"))
	if code != ExitOK {
		t.Errorf("exit = %d with every broken fixture excluded, want %d:\n%s", code, ExitOK, stdout)
	}
	if !strings.Contains(stdout, "1 file checked") {
		t.Errorf("stdout = %q, want only AGENTS.md left", stdout)
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantText string
	}{
		{"unknown format", []string{"-format", "xml", fixture("clean")}, "unknown -format"},
		{"unknown rule", []string{"-disable", "no.such.rule", fixture("clean")}, "unknown rule"},
		{"negative budget", []string{"-max-agents-tokens", "-5", fixture("clean")}, "must be zero or more"},
		{"missing path", []string{filepath.Join(fixture("clean"), "nope")}, "cannot read"},
		{"not an instruction file", []string{"cli.go"}, "not an AGENTS.md or SKILL.md"},
		{"bad exclude glob", []string{"-exclude", "[", fixture("clean")}, "invalid --exclude"},
		{"unknown flag", []string{"-nope"}, "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(tt.args...)
			if code != ExitUsage {
				t.Errorf("exit = %d, want %d (stdout: %s stderr: %s)", code, ExitUsage, stdout, stderr)
			}
			if !strings.Contains(stderr, tt.wantText) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tt.wantText)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want usage problems on stderr only", stdout)
			}
		})
	}
}

func TestVersionAndListRules(t *testing.T) {
	code, stdout, _ := run("-version")
	if code != ExitOK || !strings.HasPrefix(stdout, "ctxlint ") {
		t.Errorf("-version: exit = %d, stdout = %q", code, stdout)
	}

	code, stdout, _ = run("-list-rules")
	if code != ExitOK {
		t.Fatalf("-list-rules: exit = %d", code)
	}
	listed := strings.Fields(stdout)
	if len(listed) != len(lint.Rules) {
		t.Errorf("-list-rules printed %d rules, want %d", len(listed), len(lint.Rules))
	}
	for _, rule := range lint.Rules {
		if !strings.Contains(stdout, rule) {
			t.Errorf("-list-rules omits %s", rule)
		}
	}
}

func TestHelpExitsZero(t *testing.T) {
	code, _, stderr := run("-h")
	if code != ExitOK {
		t.Errorf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(stderr, "ctxlint lints agent instruction files") {
		t.Errorf("stderr = %q, want the usage text", stderr)
	}
}

func TestNoPathsDefaultsToWorkingDirectory(t *testing.T) {
	// This package's directory holds no instruction files, so a bare run is a
	// clean no-op rather than an error.
	code, stdout, stderr := run()
	if code != ExitOK {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, "0 files checked") {
		t.Errorf("stdout = %q, want an empty run", stdout)
	}
}

func sorted(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			return false
		}
	}
	return true
}
