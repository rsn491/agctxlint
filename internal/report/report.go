// Package report renders lint results for humans and for machines.
package report

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/rsn491/ctxlint/internal/lint"
)

// SchemaVersion is bumped when the JSON shape changes incompatibly.
const SchemaVersion = 1

// Summary counts a whole run.
type Summary struct {
	Files    int `json:"files"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// Summarize totals the findings across results.
func Summarize(results []lint.Result) Summary {
	s := Summary{Files: len(results)}
	for _, r := range results {
		s.Errors += r.Errors()
		s.Warnings += r.Warnings()
	}
	return s
}

type jsonReport struct {
	Version int           `json:"version"`
	Files   []lint.Result `json:"files"`
	Summary Summary       `json:"summary"`
}

// Reporter renders lint results to a writer in one of ctxlint's output
// formats.
type Reporter struct {
	W     io.Writer
	Quiet bool
}

// NewReporter returns a Reporter writing to w. When quiet is set, warnings
// are omitted from the rendered output (but still counted in the summary).
func NewReporter(w io.Writer, quiet bool) *Reporter {
	return &Reporter{W: w, Quiet: quiet}
}

// Text groups findings under a header naming their file, so the path is not
// repeated on every line. Files with nothing to report (or nothing left after
// quiet filtering) are omitted entirely. A blank line separates file groups
// and sets the final summary line apart.
func (r *Reporter) Text(results []lint.Result) error {
	for _, res := range results {
		findings := res.Findings
		if r.Quiet {
			findings = filterErrors(findings)
		}
		if len(findings) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(r.W, "%s\n", res.Path); err != nil {
			return err
		}
		for _, f := range findings {
			location := ""
			if f.Line > 0 {
				location = fmt.Sprintf("%d: ", f.Line)
			}
			if _, err := fmt.Fprintf(r.W, "  %s%s: %s: %s\n", location, f.Severity, f.Rule, f.Message); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(r.W); err != nil {
			return err
		}
	}

	s := Summarize(results)
	_, err := fmt.Fprintf(r.W, "%s checked, %s, %s\n",
		plural(s.Files, "file", "files"),
		plural(s.Errors, "error", "errors"),
		plural(s.Warnings, "warning", "warnings"))
	return err
}

// JSON writes the whole run as a single object, sorted by path upstream so the
// output diffs cleanly between runs.
func (r *Reporter) JSON(results []lint.Result) error {
	files := make([]lint.Result, 0, len(results))
	for _, res := range results {
		if r.Quiet {
			res.Findings = filterErrors(res.Findings)
		}
		files = append(files, res)
	}

	enc := json.NewEncoder(r.W)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonReport{
		Version: SchemaVersion,
		Files:   files,
		Summary: Summarize(results),
	})
}

func filterErrors(findings []lint.Finding) []lint.Finding {
	kept := make([]lint.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Severity != lint.SeverityWarning {
			kept = append(kept, f)
		}
	}
	return kept
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
