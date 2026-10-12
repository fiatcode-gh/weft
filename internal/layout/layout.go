// Package layout is the one definition of where a graph's pages and journals
// live: the optional .weft/config.toml, its validation, the journal filename
// pattern and the placement of new pages.
package layout

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ConfigFile is the config path relative to the graph root.
const ConfigFile = ".weft/config.toml"

// NamespaceFiles says how a namespaced page name ("a/b") maps to a file.
type NamespaceFiles string

const (
	// TripleLowbar stores "a/b" as "a___b.md" directly in the pages dir.
	TripleLowbar NamespaceFiles = "triple-lowbar"
	// Folders stores "a/b" as "a/b.md" under the pages dir.
	Folders NamespaceFiles = "folders"
)

const defaultPattern = "YYYY_MM_DD"

// Layout is a graph's validated, immutable layout. Dirs are graph-relative,
// slash-separated and cleaned; "" is the graph root.
type Layout struct {
	pagesDir    string
	journalsDir string
	namespace   NamespaceFiles
	pattern     pattern
}

// Default is the layout of a graph with no config file.
func Default() Layout {
	p, err := compilePattern(defaultPattern)
	if err != nil {
		panic(err) // the default pattern is a constant
	}
	return Layout{pagesDir: "pages", journalsDir: "journals", namespace: TripleLowbar, pattern: p}
}

type rawConfig struct {
	PagesDir           *string `toml:"pages_dir"`
	JournalsDir        *string `toml:"journals_dir"`
	JournalFilePattern *string `toml:"journal_file_pattern"`
	NamespaceFiles     *string `toml:"namespace_files"`
}

// Load reads <graphPath>/.weft/config.toml. A missing file is the default
// layout; any other problem is an error naming the config path.
func Load(graphPath string) (Layout, error) {
	cfgPath := filepath.Join(graphPath, ".weft", "config.toml")
	data, err := os.ReadFile(cfgPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Layout{}, fmt.Errorf("%s: %s", cfgPath, err)
	}
	l, err := parse(string(data))
	if err != nil {
		return Layout{}, fmt.Errorf("%s: %s", cfgPath, err)
	}
	return l, nil
}

func parse(text string) (Layout, error) {
	var raw rawConfig
	md, err := toml.Decode(text, &raw)
	if err != nil {
		return Layout{}, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		return Layout{}, fmt.Errorf("unknown key %q", und[0].String())
	}
	l := Default()
	if raw.PagesDir != nil {
		if l.pagesDir, err = cleanDir("pages_dir", *raw.PagesDir); err != nil {
			return Layout{}, err
		}
	}
	if raw.JournalsDir != nil {
		if l.journalsDir, err = cleanDir("journals_dir", *raw.JournalsDir); err != nil {
			return Layout{}, err
		}
	}
	if raw.JournalFilePattern != nil {
		p, err := compilePattern(*raw.JournalFilePattern)
		if err != nil {
			return Layout{}, fmt.Errorf("journal_file_pattern: %q %s", *raw.JournalFilePattern, err)
		}
		l.pattern = p
	}
	if raw.NamespaceFiles != nil {
		switch ns := NamespaceFiles(*raw.NamespaceFiles); ns {
		case TripleLowbar, Folders:
			l.namespace = ns
		default:
			return Layout{}, fmt.Errorf("namespace_files: %q must be %q or %q", *raw.NamespaceFiles, TripleLowbar, Folders)
		}
	}
	return l, nil
}

func cleanDir(key, v string) (string, error) {
	c := path.Clean(v)
	if path.IsAbs(v) || filepath.IsAbs(v) || c == ".." || strings.HasPrefix(c, "../") {
		return "", fmt.Errorf("%s: %q is outside the graph", key, v)
	}
	if c == "." {
		return "", nil
	}
	return c, nil
}

// PagesDir is the graph-relative pages folder ("" is the graph root).
func (l Layout) PagesDir() string { return l.pagesDir }

// JournalsDir is the graph-relative journals folder ("" is the graph root).
func (l Layout) JournalsDir() string { return l.journalsDir }

// Namespace is how namespaced page names map to files.
func (l Layout) Namespace() NamespaceFiles { return l.namespace }

// contains reports whether p is dir or lies inside it.
func contains(dir, p string) bool {
	return dir == "" || p == dir || strings.HasPrefix(p, dir+"/")
}

// Roots returns the folders to walk, outermost only, pages first.
func (l Layout) Roots() []string {
	switch {
	case contains(l.pagesDir, l.journalsDir):
		return []string{l.pagesDir}
	case contains(l.journalsDir, l.pagesDir):
		return []string{l.journalsDir}
	}
	return []string{l.pagesDir, l.journalsDir}
}

// SkipDir reports whether the walk skips the directory rel (graph-relative):
// dot folders, except the pages dir, the journals dir and their ancestors.
func (l Layout) SkipDir(rel string) bool {
	if !strings.HasPrefix(path.Base(rel), ".") {
		return false
	}
	return !contains(rel, l.pagesDir) && !contains(rel, l.journalsDir)
}

// NewPageRel returns the graph-relative path, with ".md", for a new page.
func (l Layout) NewPageRel(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("cannot place page %q: empty name", name)
	}
	file := strings.ReplaceAll(name, "/", "___")
	if l.namespace == Folders {
		segs := strings.Split(name, "/")
		for i, s := range segs {
			switch {
			case s == "" || s == "." || s == "..":
				return "", fmt.Errorf("cannot place page %q: empty, \".\" or \"..\" path segment", name)
			case i < len(segs)-1 && strings.HasPrefix(s, "."):
				return "", fmt.Errorf("cannot place page %q: folder starting with \".\"", name)
			}
		}
		file = name
	}
	return joinRel(l.pagesDir, file+".md"), nil
}

func joinRel(dir, rest string) string {
	if dir == "" {
		return rest
	}
	return dir + "/" + rest
}
