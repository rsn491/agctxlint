package parse

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSplitValid(t *testing.T) {
	src := "---\nname: my-skill\ndescription: Does a thing.\n---\n\n# Heading\n\nBody text.\n"

	fm, body, err := Split([]byte(src))
	if err != nil {
		t.Fatalf("Split returned %v, want nil", err)
	}
	if !fm.Present {
		t.Error("Present = false, want true")
	}
	if got, want := fm.Keys(), []string{"name", "description"}; !equal(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
	if got, ok := fm.String("name"); !ok || got != "my-skill" {
		t.Errorf(`String("name") = %q, %v; want "my-skill", true`, got, ok)
	}
	if got, want := fm.Line("name"), 2; got != want {
		t.Errorf(`Line("name") = %d, want %d`, got, want)
	}
	if got, want := fm.Line("description"), 3; got != want {
		t.Errorf(`Line("description") = %d, want %d`, got, want)
	}
	if got, want := fm.EndLine, 4; got != want {
		t.Errorf("EndLine = %d, want %d", got, want)
	}
	if !strings.HasPrefix(body, "\n# Heading") {
		t.Errorf("body = %q, want it to start after the closing fence", body)
	}
	if strings.Contains(body, "name:") {
		t.Errorf("body = %q, want front matter excluded", body)
	}
}

func TestSplitNoFrontmatter(t *testing.T) {
	src := "# Just markdown\n\nNo front matter here.\n"

	fm, body, err := Split([]byte(src))
	if err != nil {
		t.Fatalf("Split returned %v, want nil (absent front matter is not an error)", err)
	}
	if fm.Present {
		t.Error("Present = true, want false")
	}
	if body != src {
		t.Errorf("body = %q, want the whole file", body)
	}
}

func TestSplitBOM(t *testing.T) {
	src := "\ufeff---\nname: bom-skill\n---\nbody\n"

	fm, _, err := Split([]byte(src))
	if err != nil {
		t.Fatalf("Split returned %v, want nil", err)
	}
	if got, ok := fm.String("name"); !ok || got != "bom-skill" {
		t.Errorf(`String("name") = %q, %v; want "bom-skill", true`, got, ok)
	}
}

func TestSplitCRLF(t *testing.T) {
	src := "---\r\nname: crlf-skill\r\n---\r\nbody\r\n"

	fm, _, err := Split([]byte(src))
	if err != nil {
		t.Fatalf("Split returned %v, want nil", err)
	}
	if got, ok := fm.String("name"); !ok || got != "crlf-skill" {
		t.Errorf(`String("name") = %q, %v; want "crlf-skill", true`, got, ok)
	}
}

func TestSplitErrors(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantKind ErrKind
		wantLine int
	}{
		{
			name:     "fence not first",
			src:      "\n---\nname: late\n---\n",
			wantKind: KindNotFirst,
			wantLine: 2,
		},
		{
			name:     "unterminated",
			src:      "---\nname: open-ended\nbody without a closing fence\n",
			wantKind: KindUnterminated,
			wantLine: 1,
		},
		{
			name:     "invalid yaml",
			src:      "---\nname: ok\n  bad: indent\n---\nbody\n",
			wantKind: KindInvalidYAML,
			wantLine: 3,
		},
		{
			name:     "not a mapping",
			src:      "---\n- one\n- two\n---\nbody\n",
			wantKind: KindNotMapping,
			wantLine: 2,
		},
		{
			name:     "empty block",
			src:      "---\n---\nbody\n",
			wantKind: KindNotMapping,
			wantLine: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, body, err := Split([]byte(tt.src))
			perr, ok := err.(*Error)
			if !ok {
				t.Fatalf("Split returned %T (%v), want *Error", err, err)
			}
			if perr.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", perr.Kind, tt.wantKind)
			}
			if perr.Line != tt.wantLine {
				t.Errorf("Line = %d, want %d (message: %s)", perr.Line, tt.wantLine, perr.Msg)
			}
			if body == "" {
				t.Error("body is empty, want best-effort content so token budgets still apply")
			}
			if strings.Contains(perr.Msg, "\n") {
				t.Errorf("Msg = %q, want a single line", perr.Msg)
			}
		})
	}
}

func TestStringSlice(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
		ok   bool
	}{
		{"sequence", "---\nallowed-tools:\n  - Read\n  - Bash\n---\n", []string{"Read", "Bash"}, true},
		{"comma separated", "---\nallowed-tools: Read, Bash\n---\n", []string{"Read", "Bash"}, true},
		{"flow sequence", "---\nallowed-tools: [Read, Bash]\n---\n", []string{"Read", "Bash"}, true},
		{"mapping is rejected", "---\nallowed-tools:\n  read: true\n---\n", nil, false},
		{"nested sequence is rejected", "---\nallowed-tools:\n  - [Read]\n---\n", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, _, err := Split([]byte(tt.src))
			if err != nil {
				t.Fatalf("Split returned %v, want nil", err)
			}
			got, ok := fm.StringSlice("allowed-tools")
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && !equal(got, tt.want) {
				t.Errorf("StringSlice = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccessorsOnAbsentKeys(t *testing.T) {
	fm, _, err := Split([]byte("---\nname: only-name\n---\n"))
	if err != nil {
		t.Fatalf("Split returned %v, want nil", err)
	}
	if fm.Has("license") {
		t.Error(`Has("license") = true, want false`)
	}
	if node := fm.Node("license"); node != nil {
		t.Errorf(`Node("license") = %v, want nil`, node)
	}
	if got, ok := fm.String("license"); ok || got != "" {
		t.Errorf(`String("license") = %q, %v; want "", false`, got, ok)
	}
	if got, want := fm.Line("license"), fm.EndLine; got != want {
		t.Errorf(`Line("license") = %d, want EndLine %d`, got, want)
	}
}

func TestNodeKind(t *testing.T) {
	fm, _, err := Split([]byte("---\nmetadata:\n  owner: infra\n---\n"))
	if err != nil {
		t.Fatalf("Split returned %v, want nil", err)
	}
	if got := fm.Node("metadata"); got == nil || got.Kind != yaml.MappingNode {
		t.Errorf(`Node("metadata") kind = %v, want a mapping`, got)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
