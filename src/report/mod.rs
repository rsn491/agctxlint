//! Renders lint results for humans and for machines.

mod json;
mod text;

use std::io::{self, Write};

use crate::lint::{FileResult, Finding, Severity};

pub use json::JsonReporter;
pub use text::TextReporter;

/// Bumped when the JSON shape changes incompatibly.
pub const SCHEMA_VERSION: u32 = 1;

/// Counts a whole run.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, serde::Serialize)]
pub struct Summary {
    pub files: usize,
    pub files_with_errors: usize,
    pub files_with_warnings: usize,
}

/// Counts, across results, how many files have at least one error or
/// warning. Mirrors other linters' summaries, which report the files
/// affected rather than a raw finding tally.
pub fn summarize(results: &[FileResult]) -> Summary {
    let mut s = Summary {
        files: results.len(),
        ..Default::default()
    };
    for r in results {
        if r.errors() > 0 {
            s.files_with_errors += 1;
        }
        if r.warnings() > 0 {
            s.files_with_warnings += 1;
        }
    }
    s
}

/// One way of rendering a run.
///
/// Each implementor owns its own switches, so a call site names what it wants
/// once at construction instead of passing a row of positional bools that read
/// as `text(w, &results, false, true)` at the point of use. `w` is a trait
/// object so the reporters can be held as `dyn Report`.
pub trait Report {
    fn render(
        &self,
        w: &mut dyn Write,
        results: &[FileResult],
        summary: &Summary,
    ) -> io::Result<()>;
}

/// Drops warnings, for `--quiet`.
fn filter_errors(findings: &[Finding]) -> Vec<Finding> {
    findings
        .iter()
        .filter(|f| f.severity != Severity::Warning)
        .cloned()
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::discover::Kind;
    use crate::lint::Counts;

    use super::text::{BOLD, ERROR_EMOJI, GREEN, OK_EMOJI, RED, RESET, WARNING_EMOJI, YELLOW};

    fn render_text(results: &[FileResult], quiet: bool, color: bool) -> Vec<u8> {
        let mut buf = Vec::new();
        TextReporter { quiet, color }
            .render(&mut buf, results, &summarize(results))
            .unwrap();
        buf
    }

    fn render_json(results: &[FileResult], quiet: bool) -> Vec<u8> {
        let mut buf = Vec::new();
        JsonReporter { quiet }
            .render(&mut buf, results, &summarize(results))
            .unwrap();
        buf
    }

    fn results() -> Vec<FileResult> {
        vec![
            FileResult {
                path: "AGENTS.md".to_string(),
                kind: Kind::Agents,
                tokens: Counts {
                    content: 6142,
                    name: 0,
                    description: 0,
                },
                findings: vec![Finding {
                    file: "AGENTS.md".to_string(),
                    line: 0,
                    rule: crate::lint::RULE_TOKENS_CONTENT.to_string(),
                    severity: Severity::Error,
                    message: "content is 6,142 tokens, over the 5,000 token limit".to_string(),
                }],
            },
            FileResult {
                path: "skills/thing/SKILL.md".to_string(),
                kind: Kind::Skill,
                tokens: Counts {
                    content: 120,
                    name: 2,
                    description: 30,
                },
                findings: vec![Finding {
                    file: "skills/thing/SKILL.md".to_string(),
                    line: 2,
                    rule: crate::lint::RULE_NAME_DIR_MISMATCH.to_string(),
                    severity: Severity::Warning,
                    message: "name \"other\" does not match its directory \"thing\"".to_string(),
                }],
            },
        ]
    }

    #[test]
    fn summarize_totals_across_results() {
        assert_eq!(
            summarize(&results()),
            Summary {
                files: 2,
                files_with_errors: 1,
                files_with_warnings: 1
            }
        );
    }

    #[test]
    fn text_output_shape() {
        let buf = render_text(&results(), false, false);
        let out = String::from_utf8(buf).unwrap();
        let lines: Vec<&str> = out.trim_end_matches('\n').split('\n').collect();
        assert_eq!(lines.len(), 7, "{out}");
        assert_eq!(lines[0], "AGENTS.md");
        assert!(
            lines[1].starts_with("  error: tokens.content: "),
            "{}",
            lines[1]
        );
        assert_eq!(lines[2], "");
        assert_eq!(lines[3], "skills/thing/SKILL.md");
        assert!(
            lines[4].starts_with("  2: warning: name.dir-mismatch: "),
            "{}",
            lines[4]
        );
        assert_eq!(lines[5], "");
        assert_eq!(
            lines[6],
            "2 files checked, 1 file with errors, 1 file with warnings"
        );
    }

    #[test]
    fn text_quiet_suppresses_warnings() {
        let buf = render_text(&results(), true, false);
        let out = String::from_utf8(buf).unwrap();
        assert!(!out.contains("warning: "));
        assert!(out.contains("1 file with warnings"));
    }

    #[test]
    fn text_singular_plural() {
        let buf = render_text(&[], false, false);
        assert_eq!(
            String::from_utf8(buf).unwrap(),
            "0 files checked, 0 files with errors, 0 files with warnings\n"
        );
    }

    #[test]
    fn text_color_adds_symbols_and_sgr_codes() {
        let buf = render_text(&results(), false, true);
        let out = String::from_utf8(buf).unwrap();
        assert!(out.contains(ERROR_EMOJI), "{out}");
        assert!(out.contains(WARNING_EMOJI), "{out}");
        assert!(out.contains(RED), "{out}");
        assert!(out.contains(YELLOW), "{out}");
        assert!(out.contains(BOLD), "{out}");
        assert!(out.contains(RESET), "{out}");
        assert!(out.contains("error"), "{out}");
        assert!(out.contains("warning"), "{out}");
    }

    #[test]
    fn text_color_clean_run_shows_ok_emoji_and_green_count() {
        let buf = render_text(&[], false, true);
        let out = String::from_utf8(buf).unwrap();
        assert!(out.starts_with(OK_EMOJI), "{out}");
        assert!(out.contains(GREEN), "{out}");
        assert!(!out.contains(RED), "{out}");
        assert!(!out.contains(YELLOW), "{out}");
    }

    #[test]
    fn json_output_shape() {
        let buf = render_json(&results(), false);
        let got: serde_json::Value = serde_json::from_slice(&buf).unwrap();
        assert_eq!(got["version"], SCHEMA_VERSION);
        assert_eq!(got["files"].as_array().unwrap().len(), 2);
        assert_eq!(got["summary"]["files"], 2);
        assert_eq!(got["summary"]["files_with_errors"], 1);
        assert_eq!(got["summary"]["files_with_warnings"], 1);
        assert_eq!(got["files"][1]["tokens"]["name"], 2);
        assert!(!String::from_utf8(buf).unwrap().contains("\"line\": 0"));
    }

    #[test]
    fn json_quiet_suppresses_warnings() {
        let buf = render_json(&results(), true);
        let got: serde_json::Value = serde_json::from_slice(&buf).unwrap();
        for file in got["files"].as_array().unwrap() {
            for finding in file["findings"].as_array().unwrap() {
                assert_ne!(finding["severity"], "warning");
            }
        }
        assert_eq!(got["summary"]["files_with_warnings"], 1);
    }

    #[test]
    fn json_quiet_does_not_mutate_input() {
        let input = results();
        let buf = render_json(&input, true);

        // The rendered report drops the warning, but the caller's results are
        // left intact -- quiet is a view, not an edit.
        let got: serde_json::Value = serde_json::from_slice(&buf).unwrap();
        for file in got["files"].as_array().unwrap() {
            assert!(file["findings"].as_array().unwrap().is_empty() || file["path"] == "AGENTS.md");
        }
        assert_eq!(got["summary"]["files_with_warnings"], 1);
        assert_eq!(summarize(&input).files_with_warnings, 1);
        assert_eq!(input[1].findings.len(), 1);
    }
}
