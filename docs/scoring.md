# Score

Findings answer "did anything fire?". The score answers "how far off is this
file?", so a near-clean file and a badly broken one don't read the same, and
a repository can watch the number move over time.

Each file is rated 0–100 as the mean of three parts:

| Part | How it rates |
| --- | --- |
| Front matter | Frontmatter rules that passed ÷ frontmatter rules applied |
| Token budgets | Full marks at or under budget, falling linearly to zero at 2x budget. A skill's `name` and `description` budgets average in with its body |
| File references | 100 if every checked reference resolves, 0 if any is broken |

A part with nothing to judge is left out of the mean rather than counted as
perfect. An `AGENTS.md` is rated on two parts (no frontmatter check); a file
with no checkable references skips that part; a budget set to `0` is switched
off and doesn't count. A file with nothing applicable at all rates 100.

A rule counts once no matter how often it fires. A rule disabled via
`--disable` was never applied, so it leaves the fraction entirely — the score
reflects the policy the run was configured with, same as it does for token
budgets. `--strict` doesn't move the score: it changes severities, not
pass/fail.

The run's score is the mean of the file scores. Both appear in the text
report and as `score` fields in `--format json`. Neither affects the exit
code.
