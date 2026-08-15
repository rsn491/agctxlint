# Working in ctxlint

`ctxlint` lints agent instruction files. It is a Go CLI with no dependencies
beyond `gopkg.in/yaml.v3`.

## Checks before pushing

```sh
gofmt -l .        # must print nothing
go vet ./...
go test ./...
```

## Layout

- `main.go` — entry point only; all logic lives in `internal/cli`.
- `internal/cli` — flags, orchestration, exit codes. `Run` takes its writers as
  arguments so tests drive the whole CLI in-process.
- `internal/discover` — turns paths into targets, prunes dependency directories.
- `internal/parse` — splits YAML front matter from the body, keeping line numbers.
- `internal/tokens` — heuristic token estimator behind the `Counter` interface.
- `internal/lint` — the rules.
- `internal/report` — text and JSON renderers.

## Adding a rule

1. Add its id constant in `internal/lint/lint.go` and append it to `Rules`, which
   backs both `--list-rules` and `--disable` validation.
2. Implement the check, emitting through the `add` closure so `--disable` and
   `--strict` keep working.
3. Add a case to the table in `internal/lint/lint_test.go` asserting the exact
   rule ids the fixture produces.
4. Document it in the README's rule table.

## Conventions

- Prefer errors for anything that breaks a file's contract with the runtime, and
  warnings for stylistic mismatches. Warnings alone exit 0.
- Findings are sorted by rule order, never by the order checks happen to run in,
  so output stays stable.
- Comments explain why a check exists, not what the code does.
