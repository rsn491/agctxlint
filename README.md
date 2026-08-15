# ctxlint

A linter for agents context: `AGENTS.md`, skills, etc.; ensuring it is properly formatted and follows best practices.

Features:
- Detects broken references
- Validates skill frontmatter
- Validates token budgets for skills and AGENTS.md 

## Quick start

Install:
```sh
go install github.com/rsn491/ctxlint@latest
```

Run in your repo:
```sh
ctxlint .
```

## Usage

```sh
ctxlint [flags] [path...]
```

Paths may be files or directories. Directories are walked recursively for
`AGENTS.md` and `SKILL.md`. 

Examples:
```sh
# lint the whole repository against the default budgets
ctxlint .

# just the skills, with a tighter body budget
ctxlint --max-skill-tokens 2000 skills/

# CI: fail on warnings too, machine-readable output
ctxlint --strict --format json .
```

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `--max-agents-tokens` | `2000` | Body token budget for `AGENTS.md` (roughly Anthropic's ~200-line guidance) |
| `--max-skill-tokens` | `5000` | Body token budget for `SKILL.md` |
| `--max-skill-name-tokens` | `16` | Budget for a skill's `name` |
| `--max-skill-description-tokens` | `100` | Budget for a skill's `description` |
| `--exclude` | — | Glob of paths to skip; repeatable |
| `--disable` | — | Rule id to skip; repeatable |
| `--strict` | `false` | Treat warnings as errors |
| `--quiet` | `false` | Report errors only (the summary still counts warnings) |
| `--format` | `text` | `text` or `json` |
| `--list-rules` | — | Print every rule id and exit |
| `--version` | — | Print the version and exit |

### Rules

| Rule | Severity | Check |
| --- | --- | --- |
| `frontmatter.missing` | error | The skill has no front-matter block |
| `frontmatter.not-first` | error | The fence does not open on line 1 |
| `frontmatter.unterminated` | error | The opening fence is never closed |
| `frontmatter.invalid` | error | Not parseable YAML, or not a mapping |
| `frontmatter.unknown-key` | warning | Key outside `name`, `description`, `license`, `compatibility`, `metadata`, `allowed-tools` (the [Agent Skills spec](https://agentskills.io/specification#frontmatter) fields), plus the Claude Code extensions `disable-model-invocation` and `argument-hint` |
| `name.required` | error | `name` is present and a non-empty string |
| `name.format` | error | `name` matches `^[a-z0-9]+(-[a-z0-9]+)*$` |
| `name.length` | error | `name` is at most 64 characters |
| `name.dir-mismatch` | warning | `name` matches the containing directory |
| `description.required` | error | `description` is present and a non-empty string |
| `description.length` | error | `description` is at most 1024 characters |
| `allowed-tools.type` | error | A list of names, or a comma-separated string |
| `metadata.type` | error | A mapping when present |
| `tokens.content` | error | Body within its token budget |
| `tokens.name` | error | `name` within its token budget |
| `tokens.description` | error | `description` within its token budget |
| `file-reference.missing` | error | Every relative markdown link in the body resolves to a file that exists |

Switch any of them off by id:
```sh
ctxlint --disable name.dir-mismatch --disable frontmatter.unknown-key .
```

## Running on CI

```yaml
- name: Lint agent instruction files
  run: |
    go install github.com/rsn491/ctxlint@latest
    ctxlint --strict .
```

## Development

Build:
```sh
go build -o ctxlint .
```

Run tests:
```sh
go test ./...     
```

Format:
```sh
go vet ./...
gofmt -l .
```
