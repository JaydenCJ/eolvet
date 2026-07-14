// Package scan walks a repository tree, hands recognized files to the
// detection engine, and returns declarations in a deterministic order.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JaydenCJ/eolvet/internal/detect"
)

// Options controls a walk.
type Options struct {
	Excludes    []string // glob patterns on repo-relative paths
	MaxFileSize int64    // skip larger files; <=0 means the 1 MiB default
}

// DefaultMaxFileSize bounds how much of any single file eolvet reads —
// version declarations live in small text files, so anything bigger is
// either generated or not ours to parse.
const DefaultMaxFileSize = 1 << 20

// skipDirs are directories that never contain the *scanned repo's own*
// declarations: vendored trees, build output, virtualenvs, caches.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	"venv":         true,
	"__pycache__":  true,
}

// Walk scans root and returns every declaration found, sorted by file
// path then line so identical trees always produce identical reports.
// Hidden directories (.git, .cache, …) are skipped; hidden *files* are
// not, because .python-version and .nvmrc are exactly what we're after.
func Walk(root string, eng *detect.Engine, opts Options) ([]detect.Decl, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	maxSize := opts.MaxFileSize
	if maxSize <= 0 {
		maxSize = DefaultMaxFileSize
	}
	if !info.IsDir() {
		// Scanning a single file (eolvet scan Dockerfile) is a common
		// CI shape; excludes do not apply to an explicit argument.
		return detectOne(root, filepath.Base(root), eng, maxSize)
	}
	var decls []detect.Decl
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] || excluded(rel, opts.Excludes) {
				return filepath.SkipDir
			}
			return nil
		}
		if !eng.Matches(d.Name()) || excluded(rel, opts.Excludes) {
			return nil
		}
		found, err := detectOne(p, rel, eng, maxSize)
		if err != nil {
			return err
		}
		decls = append(decls, found...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.SliceStable(decls, func(i, j int) bool {
		if decls[i].File != decls[j].File {
			return decls[i].File < decls[j].File
		}
		if decls[i].Line != decls[j].Line {
			return decls[i].Line < decls[j].Line
		}
		return decls[i].Product < decls[j].Product
	})
	return decls, nil
}

// excluded checks a relative path against the user's exclude globs.
// A pattern also excludes everything beneath a directory it names.
func excluded(rel string, patterns []string) bool {
	for _, pat := range patterns {
		if MatchGlob(pat, rel) || MatchGlob(strings.TrimSuffix(pat, "/")+"/**", rel) {
			return true
		}
	}
	return false
}

func detectOne(p, rel string, eng *detect.Engine, maxSize int64) ([]detect.Decl, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxSize {
		return nil, nil
	}
	content, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	decls := eng.DetectFile(rel, content)
	for i := range decls {
		decls[i].File = rel
	}
	return decls, nil
}
