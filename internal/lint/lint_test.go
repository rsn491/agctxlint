package lint

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rsn491/ctxlint/internal/discover"
)

// writeSkill puts src at <dir>/<skillName>/SKILL.md so the directory-match rule
// has something realistic to check against.
func writeSkill(t *testing.T, skillName, src string) discover.Target {
	t.Helper()
	dir := filepath.Join(t.TempDir(), skillName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return discover.Target{Path: path, Kind: discover.KindSkill}
}

func ruleIDs(findings []Finding) []string {
	ids := make([]string, 0, len(findings))
	for _, f := range findings {
		ids = append(ids, f.Rule)
	}
	return ids
}

func assertRules(t *testing.T, got []Finding, want []string) {
	t.Helper()
	gotIDs := ruleIDs(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("findings = %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("findings = %v, want %v", gotIDs, want)
		}
	}
}

const validSkill = `---
name: valid-skill
description: Use this skill when you need a fixture that passes every rule.
---

# Valid skill

Body content.
`

// generousConfig disables the token budgets so front-matter rules can be tested
// in isolation.
func generousConfig() Config {
	return Config{MaxAgentsTokens: 0, MaxSkillTokens: 0, MaxSkillNameTokens: 0, MaxSkillDescriptionTokens: 0}
}

func TestSkillFrontmatterRules(t *testing.T) {
	tests := []struct {
		name      string
		skillDir  string
		src       string
		wantRules []string
	}{
		{
			name:      "valid skill has no findings",
			skillDir:  "valid-skill",
			src:       validSkill,
			wantRules: nil,
		},
		{
			name:      "missing front matter",
			skillDir:  "no-frontmatter",
			src:       "# A skill with no front matter\n\nBody.\n",
			wantRules: []string{RuleFrontmatterMissing},
		},
		{
			name:      "front matter not first",
			skillDir:  "late-frontmatter",
			src:       "\n---\nname: late-frontmatter\ndescription: Late.\n---\n",
			wantRules: []string{RuleFrontmatterNotFirst},
		},
		{
			name:      "unterminated front matter",
			skillDir:  "unterminated",
			src:       "---\nname: unterminated\ndescription: No closing fence.\n",
			wantRules: []string{RuleFrontmatterUnterminated},
		},
		{
			name:      "invalid yaml",
			skillDir:  "bad-yaml",
			src:       "---\nname: bad-yaml\n  description: wrong indent\n---\nBody.\n",
			wantRules: []string{RuleFrontmatterInvalid},
		},
		{
			name:      "front matter is a sequence",
			skillDir:  "sequence",
			src:       "---\n- name: sequence\n---\nBody.\n",
			wantRules: []string{RuleFrontmatterInvalid},
		},
		{
			name:      "name missing",
			skillDir:  "nameless",
			src:       "---\ndescription: A skill with no name.\n---\nBody.\n",
			wantRules: []string{RuleNameRequired},
		},
		{
			name:      "name empty",
			skillDir:  "empty-name",
			src:       "---\nname: \"\"\ndescription: A skill with an empty name.\n---\nBody.\n",
			wantRules: []string{RuleNameRequired},
		},
		{
			name:      "name not a scalar",
			skillDir:  "listy-name",
			src:       "---\nname:\n  - listy-name\ndescription: A skill whose name is a list.\n---\nBody.\n",
			wantRules: []string{RuleNameRequired},
		},
		{
			name:      "name has bad characters",
			skillDir:  "Bad_Name",
			src:       "---\nname: Bad_Name\ndescription: A skill with an invalid name.\n---\nBody.\n",
			wantRules: []string{RuleNameFormat},
		},
		{
			name:      "name has double hyphen",
			skillDir:  "double--hyphen",
			src:       "---\nname: double--hyphen\ndescription: A skill with a doubled hyphen.\n---\nBody.\n",
			wantRules: []string{RuleNameFormat},
		},
		{
			name:      "name too long",
			skillDir:  "long",
			src:       "---\nname: " + strings.Repeat("a", MaxNameChars+1) + "\ndescription: A skill with a very long name.\n---\nBody.\n",
			wantRules: []string{RuleNameLength, RuleNameDirMismatch},
		},
		{
			name:      "name does not match directory",
			skillDir:  "some-directory",
			src:       "---\nname: other-name\ndescription: A skill in a mismatched directory.\n---\nBody.\n",
			wantRules: []string{RuleNameDirMismatch},
		},
		{
			name:      "description missing",
			skillDir:  "no-description",
			src:       "---\nname: no-description\n---\nBody.\n",
			wantRules: []string{RuleDescriptionRequired},
		},
		{
			name:      "description empty",
			skillDir:  "blank-description",
			src:       "---\nname: blank-description\ndescription: \"   \"\n---\nBody.\n",
			wantRules: []string{RuleDescriptionRequired},
		},
		{
			name:      "description too long",
			skillDir:  "verbose",
			src:       "---\nname: verbose\ndescription: " + strings.Repeat("x", MaxDescriptionChars+1) + "\n---\nBody.\n",
			wantRules: []string{RuleDescriptionLength},
		},
		{
			name:      "unknown key",
			skillDir:  "extra-keys",
			src:       "---\nname: extra-keys\ndescription: A skill with a stray key.\nauthor: someone\n---\nBody.\n",
			wantRules: []string{RuleFrontmatterUnknownKey},
		},
		{
			name:      "known optional keys are accepted",
			skillDir:  "optional-keys",
			src:       "---\nname: optional-keys\ndescription: A skill using every optional key.\nlicense: MIT\nallowed-tools:\n  - Read\n  - Bash\nmetadata:\n  owner: infra\n---\nBody.\n",
			wantRules: nil,
		},
		{
			name:      "allowed-tools wrong type",
			skillDir:  "bad-tools",
			src:       "---\nname: bad-tools\ndescription: A skill with a malformed tool list.\nallowed-tools:\n  read: true\n---\nBody.\n",
			wantRules: []string{RuleAllowedToolsType},
		},
		{
			name:      "metadata wrong type",
			skillDir:  "bad-metadata",
			src:       "---\nname: bad-metadata\ndescription: A skill with scalar metadata.\nmetadata: not-a-mapping\n---\nBody.\n",
			wantRules: []string{RuleMetadataType},
		},
		{
			name:      "several problems are all reported",
			skillDir:  "multi",
			src:       "---\nname: Multi_Skill\nextra: true\n---\nBody.\n",
			wantRules: []string{RuleFrontmatterUnknownKey, RuleNameFormat, RuleNameDirMismatch, RuleDescriptionRequired},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := writeSkill(t, tt.skillDir, tt.src)
			res, err := New(generousConfig(), nil).File(target)
			if err != nil {
				t.Fatalf("File: %v", err)
			}
			assertRules(t, res.Findings, tt.wantRules)
		})
	}
}

func TestTokenBudgets(t *testing.T) {
	target := writeSkill(t, "budgeted", validSkill)

	t.Run("content over budget", func(t *testing.T) {
		cfg := generousConfig()
		cfg.MaxSkillTokens = 1
		res, err := New(cfg, nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, []string{RuleNameDirMismatch, RuleTokensContent})
	})

	t.Run("zero disables the content budget", func(t *testing.T) {
		cfg := generousConfig()
		cfg.MaxSkillTokens = 0
		res, err := New(cfg, nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, []string{RuleNameDirMismatch})
	})

	t.Run("name over budget", func(t *testing.T) {
		cfg := generousConfig()
		cfg.MaxSkillNameTokens = 1
		res, err := New(cfg, nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, []string{RuleNameDirMismatch, RuleTokensName})
	})

	t.Run("description over budget", func(t *testing.T) {
		cfg := generousConfig()
		cfg.MaxSkillDescriptionTokens = 2
		res, err := New(cfg, nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, []string{RuleNameDirMismatch, RuleTokensDescription})
	})

	t.Run("counts are reported even when within budget", func(t *testing.T) {
		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if res.Tokens.Content <= 0 || res.Tokens.Name <= 0 || res.Tokens.Description <= 0 {
			t.Errorf("Tokens = %+v, want every part counted", res.Tokens)
		}
	})
}

func TestPerKindContentBudget(t *testing.T) {
	body := strings.Repeat("some filler prose to spend tokens on. ", 20)
	agents := discover.Target{Path: writeFile(t, "AGENTS.md", body), Kind: discover.KindAgents}
	skill := writeSkill(t, "kinded", "---\nname: kinded\ndescription: A skill with a body.\n---\n"+body)

	cfg := generousConfig()
	cfg.MaxSkillTokens = 10000 // generous budget for the other kind
	cfg.MaxAgentsTokens = 1    // tight budget for AGENTS.md only
	linter := New(cfg, nil)

	agentsRes, err := linter.File(agents)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if agentsRes.Errors() != 1 {
		t.Errorf("AGENTS.md findings = %v, want the agents override to fire", ruleIDs(agentsRes.Findings))
	}

	skillRes, err := linter.File(skill)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	for _, f := range skillRes.Findings {
		if f.Rule == RuleTokensContent {
			t.Errorf("SKILL.md hit %s, want the agents override not to apply to skills", f.Rule)
		}
	}

	cfg.MaxAgentsTokens = 0
	cfg.MaxSkillTokens = 1
	skillRes, err = New(cfg, nil).File(skill)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if !hasRule(skillRes.Findings, RuleTokensContent) {
		t.Errorf("SKILL.md findings = %v, want the skill override to fire", ruleIDs(skillRes.Findings))
	}
}

func TestAgentsFrontmatterIsNotValidated(t *testing.T) {
	// An AGENTS.md may legitimately carry front matter with arbitrary keys, and
	// none of the skill rules should apply to it.
	src := "---\ntitle: Project instructions\nowner: infra\n---\n\n# Instructions\n\nBody.\n"
	target := discover.Target{Path: writeFile(t, "AGENTS.md", src), Kind: discover.KindAgents}

	res, err := New(generousConfig(), nil).File(target)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	assertRules(t, res.Findings, nil)

	if res.Tokens.Content == 0 {
		t.Error("content tokens = 0, want the body counted")
	}
}

func TestAgentsUnterminatedFrontmatterIsReported(t *testing.T) {
	src := "---\ntitle: Project instructions\n\n# Instructions never closed\n"
	target := discover.Target{Path: writeFile(t, "AGENTS.md", src), Kind: discover.KindAgents}

	res, err := New(generousConfig(), nil).File(target)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	assertRules(t, res.Findings, []string{RuleFrontmatterUnterminated})
}

func TestAgentsFrontmatterExcludedFromContent(t *testing.T) {
	fmSrc := "---\ntitle: A fairly wordy front matter block that costs real tokens\n---\nBody.\n"
	withFM := discover.Target{Path: writeFile(t, "AGENTS.md", fmSrc), Kind: discover.KindAgents}
	bare := discover.Target{Path: writeFile(t, "AGENTS.md", "Body.\n"), Kind: discover.KindAgents}

	linter := New(generousConfig(), nil)
	withRes, err := linter.File(withFM)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	bareRes, err := linter.File(bare)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if withRes.Tokens.Content != bareRes.Tokens.Content {
		t.Errorf("content tokens = %d with front matter and %d without, want front matter excluded",
			withRes.Tokens.Content, bareRes.Tokens.Content)
	}
}

func TestStrictPromotesWarnings(t *testing.T) {
	target := writeSkill(t, "mismatched-dir", "---\nname: other-name\ndescription: A skill in a mismatched directory.\n---\nBody.\n")

	cfg := generousConfig()
	res, err := New(cfg, nil).File(target)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if res.Errors() != 0 || res.Warnings() != 1 {
		t.Errorf("errors=%d warnings=%d, want 0 and 1", res.Errors(), res.Warnings())
	}

	cfg.Strict = true
	res, err = New(cfg, nil).File(target)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	if res.Errors() != 1 || res.Warnings() != 0 {
		t.Errorf("strict: errors=%d warnings=%d, want 1 and 0", res.Errors(), res.Warnings())
	}
}

func TestDisableSkipsRules(t *testing.T) {
	target := writeSkill(t, "mismatched-dir", "---\nname: Bad_Name\ndescription: A skill with an invalid name.\n---\nBody.\n")

	cfg := generousConfig()
	cfg.Disabled = []string{RuleNameFormat, RuleNameDirMismatch}
	res, err := New(cfg, nil).File(target)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	assertRules(t, res.Findings, nil)
}

func TestFindingsCarryFileAndLine(t *testing.T) {
	target := writeSkill(t, "lines", "---\nname: Bad_Name\ndescription: A skill with an invalid name.\n---\nBody.\n")

	res, err := New(generousConfig(), nil).File(target)
	if err != nil {
		t.Fatalf("File: %v", err)
	}
	for _, f := range res.Findings {
		if f.File != target.Path {
			t.Errorf("finding %s has file %q, want %q", f.Rule, f.File, target.Path)
		}
		if f.Rule == RuleNameFormat && f.Line != 2 {
			t.Errorf("%s reported line %d, want 2", f.Rule, f.Line)
		}
	}
}

func TestMissingFileIsAnError(t *testing.T) {
	_, err := New(generousConfig(), nil).File(discover.Target{
		Path: filepath.Join(t.TempDir(), "SKILL.md"),
		Kind: discover.KindSkill,
	})
	if err == nil {
		t.Fatal("File returned nil error for a missing file")
	}
}

func TestRulesListIsComplete(t *testing.T) {
	// Every rule id the package can emit must appear in Rules, which backs both
	// -list-rules and -disable validation.
	seen := make(map[string]bool, len(Rules))
	for _, rule := range Rules {
		if seen[rule] {
			t.Errorf("rule %q listed twice", rule)
		}
		seen[rule] = true
	}
	emitted := []string{
		RuleFrontmatterMissing, RuleFrontmatterNotFirst, RuleFrontmatterUnterminated,
		RuleFrontmatterInvalid, RuleFrontmatterUnknownKey, RuleNameRequired,
		RuleNameFormat, RuleNameLength, RuleNameDirMismatch, RuleDescriptionRequired,
		RuleDescriptionLength, RuleAllowedToolsType, RuleMetadataType,
		RuleTokensContent, RuleTokensName, RuleTokensDescription,
		RuleFileReferenceMissing,
	}
	sort.Strings(emitted)
	for _, rule := range emitted {
		if !seen[rule] {
			t.Errorf("rule %q is missing from Rules", rule)
		}
	}
}

func TestHumanize(t *testing.T) {
	for _, tt := range []struct {
		in   int
		want string
	}{{0, "0"}, {7, "7"}, {999, "999"}, {1000, "1,000"}, {6142, "6,142"}, {1234567, "1,234,567"}} {
		if got := humanize(tt.in); got != tt.want {
			t.Errorf("humanize(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, name, src string) string {
	t.Helper()
	return writeFileIn(t, t.TempDir(), name, src)
}

func writeFileIn(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestFileReferenceRule(t *testing.T) {
	t.Run("existing relative link produces no finding", func(t *testing.T) {
		dir := t.TempDir()
		writeFileIn(t, dir, "notes.md", "notes")
		src := "# Instructions\n\nSee [notes](./notes.md) for details.\n"
		target := discover.Target{Path: writeFileIn(t, dir, "AGENTS.md", src), Kind: discover.KindAgents}

		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, nil)
	})

	t.Run("missing relative link is reported", func(t *testing.T) {
		dir := t.TempDir()
		src := "# Instructions\n\nSee [notes](./notes.md) for details.\n"
		target := discover.Target{Path: writeFileIn(t, dir, "AGENTS.md", src), Kind: discover.KindAgents}

		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, []string{RuleFileReferenceMissing})
	})

	t.Run("urls, anchors and mailto links are not checked", func(t *testing.T) {
		dir := t.TempDir()
		src := "See [docs](https://example.com/x), [mail](mailto:a@example.com) and [section](#heading).\n"
		target := discover.Target{Path: writeFileIn(t, dir, "AGENTS.md", src), Kind: discover.KindAgents}

		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, nil)
	})

	t.Run("links inside fenced code blocks are not checked", func(t *testing.T) {
		dir := t.TempDir()
		src := "Example syntax:\n\n```md\n[example](./missing.md)\n```\n"
		target := discover.Target{Path: writeFileIn(t, dir, "AGENTS.md", src), Kind: discover.KindAgents}

		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, nil)
	})

	t.Run("applies to skills too", func(t *testing.T) {
		target := writeSkill(t, "with-link",
			"---\nname: with-link\ndescription: A skill that references a missing helper script.\n---\nSee [helper](./helper.sh).\n")

		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		assertRules(t, res.Findings, []string{RuleFileReferenceMissing})
	})

	t.Run("line number points at the reference", func(t *testing.T) {
		dir := t.TempDir()
		src := "# Instructions\n\nIntro line.\n\nSee [notes](./notes.md).\n"
		target := discover.Target{Path: writeFileIn(t, dir, "AGENTS.md", src), Kind: discover.KindAgents}

		res, err := New(generousConfig(), nil).File(target)
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if len(res.Findings) != 1 || res.Findings[0].Line != 5 {
			t.Fatalf("findings = %+v, want a single finding on line 5", res.Findings)
		}
	})
}
