//! The machine-readable report.

use std::borrow::Cow;
use std::io::{self, Write};

use crate::lint::FileResult;

use super::{Report, SCHEMA_VERSION, Summary, filter_errors};

#[derive(serde::Serialize)]
struct JsonReport<'a> {
    version: u32,
    files: Vec<Cow<'a, FileResult>>,
    summary: Summary,
}

/// Writes the whole run as a single object, sorted by path upstream so the
/// output diffs cleanly between runs.
pub struct JsonReporter {
    pub quiet: bool,
}

impl Report for JsonReporter {
    fn render(
        &self,
        w: &mut dyn Write,
        results: &[FileResult],
        summary: &Summary,
    ) -> io::Result<()> {
        let files: Vec<Cow<FileResult>> = results
            .iter()
            .map(|r| {
                if self.quiet {
                    Cow::Owned(FileResult {
                        path: r.path.clone(),
                        kind: r.kind,
                        tokens: r.tokens.clone(),
                        score: r.score,
                        findings: filter_errors(&r.findings),
                    })
                } else {
                    Cow::Borrowed(r)
                }
            })
            .collect();

        let report = JsonReport {
            version: SCHEMA_VERSION,
            files,
            summary: *summary,
        };
        let mut buf = serde_json::to_vec_pretty(&report).map_err(io::Error::other)?;
        buf.push(b'\n');
        w.write_all(&buf)
    }
}
