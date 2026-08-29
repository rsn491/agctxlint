//! ctxlint lints agent instruction files (AGENTS.md and SKILL.md) for
//! front-matter correctness and token budget overruns.
//!
//! The binary in `src/main.rs` is a thin wrapper over [`cli::run`], which takes
//! its writers as arguments so the whole CLI can be driven in-process. That is
//! what lets the end-to-end tests live in `tests/` rather than inside the
//! modules they exercise.

pub mod cli;
pub mod config;
pub mod discover;
pub mod fence;
pub mod lint;
pub mod parse;
pub mod report;
pub mod tokens;
pub mod utils;
