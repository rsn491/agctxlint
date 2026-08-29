//! The human-readable report.

use std::io::{self, Write};

use crate::lint::{FileResult, Finding, Severity};
use crate::utils::plural;

use super::{Report, Summary, filter_errors};

pub(super) const RED: &str = "\x1b[31m";
pub(super) const YELLOW: &str = "\x1b[33m";
pub(super) const ORANGE: &str = "\x1b[38;5;208m";
pub(super) const GREEN: &str = "\x1b[32m";
pub(super) const BOLD: &str = "\x1b[1m";
pub(super) const RESET: &str = "\x1b[0m";

pub(super) const ERROR_EMOJI: &str = "\u{274c}"; // ❌
pub(super) const WARNING_EMOJI: &str = "\u{26a0}\u{fe0f}"; // ⚠️
pub(super) const OK_EMOJI: &str = "\u{2705}"; // ✅
pub(super) const TADA_EMOJI: &str = "\u{1f389}"; // 🎉

/// The color a score bands into: green is healthy, yellow wants a look,
/// orange is a real problem, red wants work.
fn score_color(score: u8) -> &'static str {
    match score {
        90..=100 => GREEN,
        70..=89 => YELLOW,
        50..=69 => ORANGE,
        _ => RED,
    }
}

/// Colors a score by band so a report scans at a glance.
pub(super) fn paint_score(color: bool, score: u8) -> String {
    paint(color, score_color(score), &score.to_string())
}

/// Wraps `s` in an SGR color code when `color` is set; otherwise returns it
/// unchanged.
fn paint(color: bool, code: &str, s: &str) -> String {
    if color {
        format!("{code}{s}{RESET}")
    } else {
        s.to_string()
    }
}

/// Groups findings under a header naming their file and its score, so the
/// path is not repeated on every line. A clean file is still listed, header
/// alone: the score is worth reporting even when nothing fired, and a perfect
/// score is elided from the header since there's nothing to flag. `--quiet`
/// keeps the terse view, dropping files with nothing left to report. A blank
/// line separates file groups and sets the final summary line apart.
///
/// When `color` is set, severities are colorized and prefixed with a symbol;
/// otherwise the output is plain text, unchanged from earlier versions so it
/// stays friendly to grep and diffing.
pub struct TextReporter {
    pub quiet: bool,
    pub color: bool,
}

impl Report for TextReporter {
    fn render(
        &self,
        w: &mut dyn Write,
        results: &[FileResult],
        summary: &Summary,
    ) -> io::Result<()> {
        let color = self.color;
        for r in results {
            let owned;
            let findings: &[Finding] = if self.quiet {
                owned = filter_errors(&r.findings);
                &owned
            } else {
                &r.findings
            };
            if self.quiet && findings.is_empty() {
                continue;
            }
            if r.score == 100 {
                writeln!(w, "{}", paint(color, BOLD, &r.path))?;
            } else {
                writeln!(
                    w,
                    "{}  {}",
                    paint(color, BOLD, &r.path),
                    paint_score(color, r.score)
                )?;
            }
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

        let s = summary;
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
            let score = paint_score(color, s.score);
            let score = if score_color(s.score) == GREEN {
                format!("{score} {TADA_EMOJI}")
            } else {
                score
            };
            writeln!(
                w,
                "{status} {checked} checked, {errors}, {warnings}, score {score}"
            )?;
        } else {
            writeln!(
                w,
                "{} checked, {errors}, {warnings}, score {}",
                plural(s.files, "file", "files"),
                s.score
            )?;
        }
        Ok(())
    }
}
