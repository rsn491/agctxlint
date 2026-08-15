//! ctxlint lints agent instruction files (AGENTS.md and SKILL.md) for
//! front-matter correctness and token budget overruns.

mod cli;
mod discover;
mod lint;
mod parse;
mod report;
mod tokens;

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let code = cli::run(&args, &mut std::io::stdout(), &mut std::io::stderr());
    std::process::exit(code);
}
