//! Renders lint results for humans and for machines.

use std::io::{self, Write};

use crate::lint::{FileResult, Finding, Severity};

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

#[derive(serde::Serialize)]
struct JsonReport<'a> {
    version: u32,
    files: Vec<std::borrow::Cow<'a, FileResult>>,
    summary: Summary,
}

const RED: &str = "\x1b[31m";
const YELLOW: &str = "\x1b[33m";
const GREEN: &str = "\x1b[32m";
const BOLD: &str = "\x1b[1m";
const RESET: &str = "\x1b[0m";

const ERROR_EMOJI: &str = "\u{274c}"; // ❌
const WARNING_EMOJI: &str = "\u{26a0}\u{fe0f}"; // ⚠️
const OK_EMOJI: &str = "\u{2705}"; // ✅

/// Wraps `s` in an SGR color code when `color` is set; otherwise returns it
/// unchanged.
fn paint(color: bool, code: &str, s: &str) -> String {
    if color {
        format!("{code}{s}{RESET}")
    } else {
        s.to_string()
    }
}

/// Groups findings under a header naming their file, so the path is not
/// repeated on every line. Files with nothing to report (or nothing left
/// after quiet filtering) are omitted entirely. A blank line separates file
/// groups and sets the final summary line apart.
///
/// When `color` is set, severities are colorized and prefixed with a symbol;
/// otherwise the output is plain text, unchanged from earlier versions so it
/// stays friendly to grep and diffing.
pub fn text(
    w: &mut impl Write,
    results: &[FileResult],
    quiet: bool,
    color: bool,
) -> io::Result<()> {
    for r in results {
        let owned;
        let findings: &[Finding] = if quiet {
            owned = filter_errors(&r.findings);
            &owned
        } else {
            &r.findings
        };
        if findings.is_empty() {
            continue;
        }
        writeln!(w, "{}", paint(color, BOLD, &r.path))?;
        for f in findings {
            let location = if f.line > 0 {
                format!("{}: ", f.line)
            } else {
                String::new()
            };
            let (emoji, code) = match f.severity {
                Severity::Error => (ERROR_EMOJI, RED),
                Severity::Warning => (WARNING_EMOJI, YELLOW),
            };
            let severity = paint(color, code, &f.severity.to_string());
            if color {
                writeln!(
                    w,
                    "  {location}{emoji} {severity}: {}: {}",
                    f.rule, f.message
                )?;
            } else {
                writeln!(w, "  {location}{severity}: {}: {}", f.rule, f.message)?;
            }
        }
        writeln!(w)?;
    }

    let s = summarize(results);
    let errors = plural(s.files_with_errors, "file with errors", "files with errors");
    let warnings = plural(
        s.files_with_warnings,
        "file with warnings",
        "files with warnings",
    );
    let errors = if s.files_with_errors > 0 {
        paint(color, RED, &errors)
    } else {
        errors
    };
    let warnings = if s.files_with_warnings > 0 {
        paint(color, YELLOW, &warnings)
    } else {
        warnings
    };

    if color {
        let clean = s.files_with_errors == 0 && s.files_with_warnings == 0;
        let status = if s.files_with_errors > 0 {
            ERROR_EMOJI
        } else if s.files_with_warnings > 0 {
            WARNING_EMOJI
        } else {
            OK_EMOJI
        };
        let checked = plural(s.files, "file", "files");
        let checked = if clean {
            paint(color, GREEN, &checked)
        } else {
            checked
        };
        writeln!(w, "{status} {checked} checked, {errors}, {warnings}")?;
    } else {
        writeln!(
            w,
            "{} checked, {errors}, {warnings}",
            plural(s.files, "file", "files")
        )?;
    }
    Ok(())
}

/// Writes the whole run as a single object, sorted by path upstream so the
/// output diffs cleanly between runs.
pub fn json(w: &mut impl Write, results: &[FileResult], quiet: bool) -> io::Result<()> {
    let files: Vec<std::borrow::Cow<FileResult>> = results
        .iter()
        .map(|r| {
            if quiet {
                std::borrow::Cow::Owned(FileResult {
                    path: r.path.clone(),
                    kind: r.kind,
                    tokens: r.tokens.clone(),
                    findings: filter_errors(&r.findings),
                })
            } else {
                std::borrow::Cow::Borrowed(r)
            }
        })
        .collect();

    let report = JsonReport {
        version: SCHEMA_VERSION,
        files,
        summary: summarize(results),
    };
    let mut buf = serde_json::to_vec_pretty(&report).map_err(io::Error::other)?;
    buf.push(b'\n');
    w.write_all(&buf)
}

fn filter_errors(findings: &[Finding]) -> Vec<Finding> {
    findings
        .iter()
        .filter(|f| f.severity != Severity::Warning)
        .cloned()
        .collect()
}

fn plural(n: usize, one: &str, many: &str) -> String {
    if n == 1 {
        format!("{n} {one}")
    } else {
        format!("{n} {many}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::discover::Kind;
    use crate::lint::Counts;

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
        let mut buf = Vec::new();
        text(&mut buf, &results(), false, false).unwrap();
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
        let mut buf = Vec::new();
        text(&mut buf, &results(), true, false).unwrap();
        let out = String::from_utf8(buf).unwrap();
        assert!(!out.contains("warning: "));
        assert!(out.contains("1 file with warnings"));
    }

    #[test]
    fn text_singular_plural() {
        let mut buf = Vec::new();
        text(&mut buf, &[], false, false).unwrap();
        assert_eq!(
            String::from_utf8(buf).unwrap(),
            "0 files checked, 0 files with errors, 0 files with warnings\n"
        );
    }

    #[test]
    fn text_color_adds_symbols_and_sgr_codes() {
        let mut buf = Vec::new();
        text(&mut buf, &results(), false, true).unwrap();
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
        let mut buf = Vec::new();
        text(&mut buf, &[], false, true).unwrap();
        let out = String::from_utf8(buf).unwrap();
        assert!(out.starts_with(OK_EMOJI), "{out}");
        assert!(out.contains(GREEN), "{out}");
        assert!(!out.contains(RED), "{out}");
        assert!(!out.contains(YELLOW), "{out}");
    }

    #[test]
    fn json_output_shape() {
        let mut buf = Vec::new();
        json(&mut buf, &results(), false).unwrap();
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
        let mut buf = Vec::new();
        json(&mut buf, &results(), true).unwrap();
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
        let mut buf = Vec::new();
        json(&mut buf, &input, true).unwrap();
        assert_eq!(summarize(&input).files_with_warnings, 1);
    }
}
