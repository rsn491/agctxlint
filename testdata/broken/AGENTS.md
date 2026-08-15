# Broken fixtures

An AGENTS.md with nothing wrong with it structurally. Tests point a deliberately
tiny `-max-agents-tokens` at this file to exercise the content budget without
committing a twenty-kilobyte fixture.

## Notes

Front matter is optional in AGENTS.md, and when it is present its keys are not
validated against the skill spec.
