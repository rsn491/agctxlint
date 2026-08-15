package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree writes files (relative path -> contents) under a fresh temp dir and
// returns the root.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return root
}

// relPaths reduces targets to root-relative slash paths so expectations stay
// readable.
func relPaths(t *testing.T, root string, targets []Target) []string {
	t.Helper()
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		rel, err := filepath.Rel(root, filepath.FromSlash(target.Path))
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		out = append(out, filepath.ToSlash(rel)+" ("+string(target.Kind)+")")
	}
	return out
}

func TestKindFor(t *testing.T) {
	tests := []struct {
		path     string
		wantKind Kind
		wantOK   bool
	}{
		{"AGENTS.md", KindAgents, true},
		{"agents.md", KindAgents, true},
		{"a/b/Agents.MD", KindAgents, true},
		{"SKILL.md", KindSkill, true},
		{"skill.md", KindSkill, true},
		{"SKILLS.md", "", false},
		{"README.md", "", false},
		{"AGENT.md", "", false},
		{"agents.markdown", "", false},
		{"AGENTS.md.bak", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			kind, ok := KindFor(tt.path)
			if ok != tt.wantOK || kind != tt.wantKind {
				t.Errorf("KindFor(%q) = %q, %v; want %q, %v", tt.path, kind, ok, tt.wantKind, tt.wantOK)
			}
		})
	}
}

func TestFindWalksRecursively(t *testing.T) {
	root := tree(t, map[string]string{
		"AGENTS.md":                     "root instructions",
		"README.md":                     "not an instruction file",
		"docs/AGENTS.md":                "nested instructions",
		"skills/alpha/SKILL.md":         "alpha",
		"skills/beta/skill.md":          "beta, lowercase",
		"skills/gamma/SKILLS.md":        "gamma, plural, not matched",
		"node_modules/pkg/SKILL.md":     "dependency skill",
		".git/hooks/AGENTS.md":          "git internals",
		"vendor/dep/AGENTS.md":          "vendored",
		"dist/AGENTS.md":                "build output",
		"skills/alpha/reference/doc.md": "supporting doc",
	})

	targets, err := Find([]string{root}, nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	want := []string{
		"AGENTS.md (agents)",
		"docs/AGENTS.md (agents)",
		"skills/alpha/SKILL.md (skill)",
		"skills/beta/skill.md (skill)",
	}
	got := relPaths(t, root, targets)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Find =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFindExcludes(t *testing.T) {
	root := tree(t, map[string]string{
		"AGENTS.md":             "root",
		"examples/AGENTS.md":    "example",
		"skills/keep/SKILL.md":  "keep",
		"skills/drop/SKILL.md":  "drop",
		"fixtures/bad/SKILL.md": "fixture",
	})

	tests := []struct {
		name     string
		excludes []string
		want     []string
	}{
		{
			name:     "directory name",
			excludes: []string{"examples"},
			want: []string{
				"AGENTS.md (agents)",
				"fixtures/bad/SKILL.md (skill)",
				"skills/drop/SKILL.md (skill)",
				"skills/keep/SKILL.md (skill)",
			},
		},
		{
			name:     "relative path glob",
			excludes: []string{"skills/drop"},
			want: []string{
				"AGENTS.md (agents)",
				"examples/AGENTS.md (agents)",
				"fixtures/bad/SKILL.md (skill)",
				"skills/keep/SKILL.md (skill)",
			},
		},
		{
			name:     "several patterns",
			excludes: []string{"examples", "fixtures"},
			want: []string{
				"AGENTS.md (agents)",
				"skills/drop/SKILL.md (skill)",
				"skills/keep/SKILL.md (skill)",
			},
		},
		{
			name:     "file base name",
			excludes: []string{"AGENTS.md"},
			want: []string{
				"fixtures/bad/SKILL.md (skill)",
				"skills/drop/SKILL.md (skill)",
				"skills/keep/SKILL.md (skill)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets, err := Find([]string{root}, tt.excludes)
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			got := relPaths(t, root, targets)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("Find =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestFindExplicitFile(t *testing.T) {
	root := tree(t, map[string]string{
		"nested/deep/SKILL.md": "a skill somewhere",
		"README.md":            "not an instruction file",
	})

	targets, err := Find([]string{filepath.Join(root, "nested/deep/SKILL.md")}, nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(targets) != 1 || targets[0].Kind != KindSkill {
		t.Fatalf("Find = %+v, want one skill target", targets)
	}

	// An explicit file inside a skipped directory is still linted: the skip list
	// only prunes walks.
	skipped := tree(t, map[string]string{"node_modules/pkg/SKILL.md": "dependency skill"})
	targets, err = Find([]string{filepath.Join(skipped, "node_modules/pkg/SKILL.md")}, nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("Find = %+v, want the explicitly named file", targets)
	}
}

func TestFindErrors(t *testing.T) {
	root := tree(t, map[string]string{"README.md": "not an instruction file"})

	t.Run("unrecognized file name", func(t *testing.T) {
		_, err := Find([]string{filepath.Join(root, "README.md")}, nil)
		if err == nil || !strings.Contains(err.Error(), "not an AGENTS.md or SKILL.md") {
			t.Errorf("Find error = %v, want a complaint about the file name", err)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := Find([]string{filepath.Join(root, "nope")}, nil)
		if err == nil {
			t.Error("Find error = nil, want a read failure")
		}
	})

	t.Run("invalid exclude glob", func(t *testing.T) {
		_, err := Find([]string{root}, []string{"["})
		if err == nil || !strings.Contains(err.Error(), "invalid --exclude") {
			t.Errorf("Find error = %v, want a complaint about the pattern", err)
		}
	})
}

func TestFindDeduplicates(t *testing.T) {
	root := tree(t, map[string]string{"AGENTS.md": "root"})
	file := filepath.Join(root, "AGENTS.md")

	targets, err := Find([]string{root, file, file}, nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(targets) != 1 {
		t.Errorf("Find = %+v, want one target after deduplication", targets)
	}
}

func TestFindEmptyResult(t *testing.T) {
	root := tree(t, map[string]string{"README.md": "nothing to lint here"})

	targets, err := Find([]string{root}, nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(targets) != 0 {
		t.Errorf("Find = %+v, want no targets", targets)
	}
}
