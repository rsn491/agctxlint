# Score

Each file is rated 0–100 as the mean of three parts:

| Part | How it rates |
| --- | --- |
| Front matter | Frontmatter rules that passed ÷ frontmatter rules applied |
| Token budgets | 100 if within the budget, falling linearly to zero at 2x budget. A skill's `name` and `description` budgets average in with its body |
| File references | 100 if every checked reference resolves, 0 if any is broken |

A rule counts once no matter how often it fires. A rule disabled via
`--disable` isn't counted at all, so the score reflects the policy the run
was configured with — same as it does for token budgets. `--strict` doesn't
move the score: it changes severities, not pass/fail.

The run's score is the mean of the file scores. Both appear in the text
report and as `score` fields in `--format json`. Neither affects the exit
code.
