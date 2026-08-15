// Package parse splits a markdown file into YAML front matter and body, keeping
// enough position information to anchor lint findings to the right line.
package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrKind classifies a malformed front-matter block. The linter maps each kind
// onto a rule id.
type ErrKind string

const (
	// KindNotFirst means the fence exists but something precedes it.
	KindNotFirst ErrKind = "not-first"
	// KindUnterminated means the opening fence is never closed.
	KindUnterminated ErrKind = "unterminated"
	// KindInvalidYAML means the block is not parseable YAML.
	KindInvalidYAML ErrKind = "invalid-yaml"
	// KindNotMapping means the block parses but is not a key/value mapping.
	KindNotMapping ErrKind = "not-mapping"
)

// Error describes why a front-matter block could not be used.
type Error struct {
	Kind ErrKind
	Line int // 1-based line in the file, 0 when unknown
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// Frontmatter is a parsed front-matter block.
type Frontmatter struct {
	// Present reports whether the file opened with a front-matter fence.
	Present bool
	// EndLine is the 1-based line of the closing fence, 0 when absent.
	EndLine int

	keys    []string
	entries map[string]entry
}

type entry struct {
	keyLine int
	value   *yaml.Node
}

// Keys returns the mapping keys in document order.
func (f Frontmatter) Keys() []string { return f.keys }

// Has reports whether key is present, regardless of its value.
func (f Frontmatter) Has(key string) bool {
	_, ok := f.entries[key]
	return ok
}

// Line returns the 1-based line where key is declared, or EndLine as a
// fallback when the key is absent.
func (f Frontmatter) Line(key string) int {
	if e, ok := f.entries[key]; ok {
		return e.keyLine
	}
	return f.EndLine
}

// Node returns the value node for key, or nil when the key is absent.
func (f Frontmatter) Node(key string) *yaml.Node {
	if e, ok := f.entries[key]; ok {
		return e.value
	}
	return nil
}

// String returns the scalar value of key. ok is false when the key is absent or
// its value is not a scalar.
func (f Frontmatter) String(key string) (value string, ok bool) {
	n := f.Node(key)
	if n == nil || n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

// StringSlice returns the value of key as a list of strings, accepting either a
// YAML sequence of scalars or a single comma-separated scalar. ok is false when
// the key is absent or holds neither shape.
func (f Frontmatter) StringSlice(key string) (values []string, ok bool) {
	n := f.Node(key)
	if n == nil {
		return nil, false
	}
	switch n.Kind {
	case yaml.ScalarNode:
		for _, part := range strings.Split(n.Value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				values = append(values, part)
			}
		}
		return values, true
	case yaml.SequenceNode:
		for _, item := range n.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, false
			}
			values = append(values, item.Value)
		}
		return values, true
	default:
		return nil, false
	}
}

const bom = "\ufeff"

var (
	openFenceRE  = regexp.MustCompile(`^-{3,}[ \t]*$`)
	closeFenceRE = regexp.MustCompile(`^(-{3,}|\.{3,})[ \t]*$`)
)

func fenceLine(line string) string { return strings.TrimRight(line, "\r") }

// Split separates the front matter of a markdown file from its body.
//
// A file with no front matter is not an error: Frontmatter.Present is false and
// body is the whole file. A malformed block yields an *Error and a best-effort
// body so token budgets can still be reported.
func Split(src []byte) (Frontmatter, string, error) {
	text := strings.TrimPrefix(string(src), bom)
	lines := strings.Split(text, "\n")

	open := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if openFenceRE.MatchString(fenceLine(line)) {
			open = i
		}
		break // only the first non-blank line can open front matter
	}
	if open == -1 {
		return Frontmatter{}, text, nil
	}
	if open > 0 {
		return Frontmatter{}, text, &Error{
			Kind: KindNotFirst,
			Line: open + 1,
			Msg:  "front matter must start on the first line of the file",
		}
	}

	closeIdx := -1
	for i := open + 1; i < len(lines); i++ {
		if closeFenceRE.MatchString(fenceLine(lines[i])) {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return Frontmatter{Present: true}, text, &Error{
			Kind: KindUnterminated,
			Line: open + 1,
			Msg:  "front matter is not closed by a --- fence",
		}
	}

	block := strings.Join(lines[open+1:closeIdx], "\n")
	body := ""
	if closeIdx+1 < len(lines) {
		body = strings.Join(lines[closeIdx+1:], "\n")
	}

	fm := Frontmatter{Present: true, EndLine: closeIdx + 1}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(block), &doc); err != nil {
		return fm, body, &Error{
			Kind: KindInvalidYAML,
			// yaml reports lines relative to the block, which starts one line
			// after the opening fence.
			Line: yamlErrLine(err) + open + 1,
			Msg:  fmt.Sprintf("invalid YAML in front matter: %s", cleanYAMLErr(err)),
		}
	}

	// An empty block decodes to a zero node with no content.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return fm, body, &Error{
			Kind: KindNotMapping,
			Line: open + 1,
			Msg:  "front matter is empty",
		}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fm, body, &Error{
			Kind: KindNotMapping,
			Line: root.Line + open + 1,
			Msg:  "front matter must be a mapping of keys to values",
		}
	}

	fm.entries = make(map[string]entry, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		if _, dup := fm.entries[key.Value]; !dup {
			fm.keys = append(fm.keys, key.Value)
		}
		fm.entries[key.Value] = entry{keyLine: key.Line + open + 1, value: value}
	}
	return fm, body, nil
}

var yamlLineRE = regexp.MustCompile(`line (\d+)`)

// yamlErrLine pulls the 1-based line out of a yaml error message, returning 0
// when the message carries no position.
func yamlErrLine(err error) int {
	m := yamlLineRE.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n, convErr := strconv.Atoi(m[1])
	if convErr != nil {
		return 0
	}
	return n
}

// cleanYAMLErr trims yaml.v3's prefixes and collapses multi-line messages so a
// finding stays on one line.
func cleanYAMLErr(err error) string {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	msg = strings.TrimPrefix(msg, "unmarshal errors:\n  ")
	msg = strings.ReplaceAll(msg, "\n", "; ")
	return strings.TrimSpace(msg)
}
