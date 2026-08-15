// Package discover turns command-line paths into the set of agent instruction
// files to lint.
package discover

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Kind distinguishes the two file types ctxlint understands.
type Kind string

const (
	// KindAgents is an AGENTS.md instruction file.
	KindAgents Kind = "agents"
	// KindSkill is a SKILL.md skill definition.
	KindSkill Kind = "skill"
)

// Target is a single file to lint.
type Target struct {
	Path string
	Kind Kind
}

// skipDirs are never walked: they hold dependencies and build output, whose
// instruction files are not the user's to fix.
var skipDirs = map[string]bool{
	".git":         true,
	".next":        true,
	".venv":        true,
	"build":        true,
	"dist":         true,
	"node_modules": true,
	"target":       true,
	"vendor":       true,
}

// KindFor classifies a path by its base name, case-insensitively. ok is false
// when the name is not an agent instruction file.
func KindFor(path string) (kind Kind, ok bool) {
	switch strings.ToLower(filepath.Base(path)) {
	case "agents.md":
		return KindAgents, true
	case "skill.md":
		return KindSkill, true
	default:
		return "", false
	}
}

// Find collects the files to lint from the given paths. A file argument is
// taken as-is; a directory is walked recursively. Excludes are shell globs
// matched against each entry's base name and its path relative to the walk
// root, and a matching directory is pruned.
//
// The result is deduplicated by absolute path and sorted, so output is stable
// across runs and platforms.
func Find(paths []string, excludes []string) ([]Target, error) {
	if err := validateGlobs(excludes); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var targets []Target
	add := func(path string, kind Kind) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if seen[abs] {
			return
		}
		seen[abs] = true
		targets = append(targets, Target{Path: filepath.ToSlash(path), Kind: kind})
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", path, err)
		}
		if !info.IsDir() {
			kind, ok := KindFor(path)
			if !ok {
				return nil, fmt.Errorf("%s is not an AGENTS.md or SKILL.md", path)
			}
			add(path, kind)
			continue
		}
		if err := walk(path, excludes, add); err != nil {
			return nil, err
		}
	}

	sort.Slice(targets, func(i, j int) bool { return targets[i].Path < targets[j].Path })
	return targets, nil
}

func walk(root string, excludes []string, add func(string, Kind)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", path, err)
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if skipDirs[d.Name()] || matchesAny(excludes, d.Name(), rel) {
				return fs.SkipDir
			}
			return nil
		}
		kind, ok := KindFor(path)
		if !ok || matchesAny(excludes, d.Name(), rel) {
			return nil
		}
		add(path, kind)
		return nil
	})
}

func matchesAny(globs []string, name, rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, glob := range globs {
		if ok, _ := filepath.Match(glob, name); ok {
			return true
		}
		if ok, _ := filepath.Match(glob, rel); ok {
			return true
		}
	}
	return false
}

// validateGlobs rejects malformed patterns up front rather than silently
// matching nothing on every path.
func validateGlobs(globs []string) error {
	for _, glob := range globs {
		if _, err := filepath.Match(glob, ""); err != nil {
			return fmt.Errorf("invalid --exclude pattern %q: %w", glob, err)
		}
	}
	return nil
}
