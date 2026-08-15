package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rsn491/ctxlint/internal/discover"
	"github.com/rsn491/ctxlint/internal/lint"
)

func results() []lint.Result {
	return []lint.Result{
		{
			Path:   "AGENTS.md",
			Kind:   discover.KindAgents,
			Tokens: lint.Counts{Content: 6142},
			Findings: []lint.Finding{{
				File:     "AGENTS.md",
				Rule:     lint.RuleTokensContent,
				Severity: lint.SeverityError,
				Message:  "content is 6,142 tokens, over the 5,000 token limit",
			}},
		},
		{
			Path:   "skills/thing/SKILL.md",
			Kind:   discover.KindSkill,
			Tokens: lint.Counts{Content: 120, Name: 2, Description: 30},
			Findings: []lint.Finding{{
				File:     "skills/thing/SKILL.md",
				Line:     2,
				Rule:     lint.RuleNameDirMismatch,
				Severity: lint.SeverityWarning,
				Message:  `name "other" does not match its directory "thing"`,
			}},
		},
	}
}

func TestSummarize(t *testing.T) {
	got := Summarize(results())
	want := Summary{Files: 2, Errors: 1, Warnings: 1}
	if got != want {
		t.Errorf("Summarize = %+v, want %+v", got, want)
	}
}

func TestText(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, results(), false); err != nil {
		t.Fatalf("Text: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("output has %d lines, want 2 file groups (header, finding, blank) and a summary:\n%s", len(lines), buf.String())
	}
	if lines[0] != "AGENTS.md" {
		t.Errorf("line 0 = %q, want the file header %q", lines[0], "AGENTS.md")
	}
	// A finding with no line number omits the line prefix entirely.
	if want := "  error: tokens.content: "; !strings.HasPrefix(lines[1], want) {
		t.Errorf("line 1 = %q, want it to start with %q", lines[1], want)
	}
	if lines[2] != "" {
		t.Errorf("line 2 = %q, want a blank line between file groups", lines[2])
	}
	if lines[3] != "skills/thing/SKILL.md" {
		t.Errorf("line 3 = %q, want the file header %q", lines[3], "skills/thing/SKILL.md")
	}
	if want := "  2: warning: name.dir-mismatch: "; !strings.HasPrefix(lines[4], want) {
		t.Errorf("line 4 = %q, want it to start with %q", lines[4], want)
	}
	if lines[5] != "" {
		t.Errorf("line 5 = %q, want a blank line before the summary", lines[5])
	}
	if want := "2 files checked, 1 error, 1 warning"; lines[6] != want {
		t.Errorf("summary = %q, want %q", lines[6], want)
	}
}

func TestTextQuiet(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, results(), true); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if strings.Contains(buf.String(), "warning: ") {
		t.Errorf("output = %q, want warnings suppressed", buf.String())
	}
	// The summary still reports what was suppressed.
	if !strings.Contains(buf.String(), "1 warning") {
		t.Errorf("output = %q, want the warning counted in the summary", buf.String())
	}
}

func TestTextSingularPlural(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, nil, false); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if want := "0 files checked, 0 errors, 0 warnings\n"; buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, results(), false); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got jsonReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if got.Version != SchemaVersion {
		t.Errorf("version = %d, want %d", got.Version, SchemaVersion)
	}
	if len(got.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(got.Files))
	}
	if got.Summary != (Summary{Files: 2, Errors: 1, Warnings: 1}) {
		t.Errorf("summary = %+v, want 2 files, 1 error, 1 warning", got.Summary)
	}
	if got.Files[1].Tokens.Name != 2 {
		t.Errorf("skill name tokens = %d, want 2", got.Files[1].Tokens.Name)
	}
	// A zero line is omitted rather than reported as line 0.
	if strings.Contains(buf.String(), `"line": 0`) {
		t.Errorf("output contains a zero line number:\n%s", buf.String())
	}
}

func TestJSONQuiet(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, results(), true); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got jsonReport
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	for _, file := range got.Files {
		for _, finding := range file.Findings {
			if finding.Severity == lint.SeverityWarning {
				t.Errorf("%s reports warning %s, want it suppressed", file.Path, finding.Rule)
			}
		}
	}
	if got.Summary.Warnings != 1 {
		t.Errorf("summary.warnings = %d, want the suppressed warning still counted", got.Summary.Warnings)
	}
}

// TestJSONQuietDoesNotMutateInput guards against the filtering in JSON reaching
// back into the caller's results, which would corrupt a later text render.
func TestJSONQuietDoesNotMutateInput(t *testing.T) {
	in := results()
	if err := JSON(&bytes.Buffer{}, in, true); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got := Summarize(in); got.Warnings != 1 {
		t.Errorf("input warnings = %d after a quiet render, want 1", got.Warnings)
	}
}
