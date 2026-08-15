# Working in ctxlint

`ctxlint` lints agent instruction files. It is a Rust CLI; the YAML front
matter is parsed with `saphyr`, everything else is hand-rolled.

## Checks before pushing

```sh
cargo fmt --check       # must report nothing
cargo clippy --all-targets -- -D warnings
cargo test
```

## Layout

- `src/main.rs` — entry point only; all logic lives in the other modules.
- `src/cli.rs` — flags, orchestration, exit codes. `run` takes its writers as
  arguments so tests drive the whole CLI in-process.
- `src/discover.rs` — turns paths into targets, prunes dependency directories.
- `src/parse.rs` — splits YAML front matter from the body, keeping line numbers.
- `src/tokens.rs` — heuristic token estimator behind the `Counter` trait.
- `src/lint.rs` — the rules.
- `src/report.rs` — text and JSON renderers.

## Adding a rule

1. Add its id constant in `src/lint.rs` and append it to `RULES`, which backs
   both `--list-rules` and `--disable` validation.
2. Implement the check, emitting through the `Reporter::add` method so
   `--disable` and `--strict` keep working.
3. Add a case to the table in `lint.rs`'s test module asserting the exact rule
   ids the fixture produces.
4. Document it in the README's rule table.

## Conventions

- Prefer errors for anything that breaks a file's contract with the runtime, and
  warnings for stylistic mismatches. Warnings alone exit 0.
- Findings are sorted by rule order, never by the order checks happen to run in,
  so output stays stable.
- Comments explain why a check exists, not what the code does.
