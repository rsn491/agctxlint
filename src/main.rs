//! Command-line entry point. All logic lives in the library.

use std::io::IsTerminal;

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let is_terminal = std::io::stdout().is_terminal();
    let code = ctxlint::cli::run(
        &args,
        &mut std::io::stdout(),
        &mut std::io::stderr(),
        is_terminal,
    );
    std::process::exit(code);
}
