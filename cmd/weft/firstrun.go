package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fiatcode-gh/weft/v3/internal/edit"
	"github.com/fiatcode-gh/weft/v3/internal/userconfig"
)

// errNoAnswer reports that input ended at a first-run prompt.
var errNoAnswer = errors.New("no graph folder chosen; nothing was saved")

// pickGraph is main's no-graph branch. On a terminal (interactive) with a known user config
// path and home folder it runs firstRun; otherwise it returns noGraphError() and touches nothing.
func pickGraph(interactive bool, in io.Reader, out io.Writer, seen edit.Snapshot) (graphChoice, error) {
	cfgPath := userconfig.Path()
	home, _ := os.UserHomeDir()
	if !interactive || cfgPath == "" || home == "" {
		return graphChoice{}, noGraphError()
	}
	path, err := firstRun(in, out, cfgPath, home, seen)
	if err != nil {
		return graphChoice{}, err
	}
	return graphChoice{path, cfgPath}, nil
}

// firstRun asks for the graph folder until a usable one is chosen, creates it after a yes,
// saves it as the user config's graph key (guarded against seen) and returns the folder.
func firstRun(in io.Reader, out io.Writer, cfgPath, home string, seen edit.Snapshot) (string, error) {
	r := bufio.NewReader(in)
	fmt.Fprint(out, "weft keeps your notes in a folder, the graph.\n")
	abs, err := chooseFolder(r, out, home)
	if err != nil {
		return "", err
	}
	value := userconfig.GraphValue(abs, home)
	if err := userconfig.SaveFirstRun(cfgPath, seen, value, nil); err != nil {
		if errors.Is(err, edit.ErrChanged) {
			return "", fmt.Errorf("%s changed while weft was asking; graph not saved", cfgPath)
		}
		return "", fmt.Errorf("save graph to %s: %w", cfgPath, err)
	}
	fmt.Fprintf(out, "Saved graph = %q to %s\n", value, cfgPath)
	return abs, nil
}

// chooseFolder loops until the user names a folder that exists or was created.
func chooseFolder(r *bufio.Reader, out io.Writer, home string) (string, error) {
	for {
		fmt.Fprint(out, "Notes folder [~/notes]: ")
		answer, err := readAnswer(r, out)
		if err != nil {
			return "", err
		}
		abs, ok := resolveAnswer(answer, home)
		if !ok {
			fmt.Fprint(out, "Use ~/ or a full path.\n")
			continue
		}
		info, err := os.Stat(abs)
		switch {
		case err == nil && info.IsDir():
			return abs, nil
		case err == nil:
			fmt.Fprintf(out, "%s is a file. Choose a folder.\n", abs)
		case errors.Is(err, fs.ErrNotExist):
			create, err := confirmCreate(r, out, abs)
			if err != nil {
				return "", err
			}
			if !create {
				continue
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				fmt.Fprintf(out, "Cannot create %s: %v\n", abs, err)
				continue
			}
			return abs, nil
		default:
			fmt.Fprintf(out, "Cannot use %s: %v\n", abs, err)
		}
	}
}

// confirmCreate asks until it gets a yes or a no.
func confirmCreate(r *bufio.Reader, out io.Writer, abs string) (bool, error) {
	for {
		fmt.Fprintf(out, "Create %s? [Y/n]: ", abs)
		answer, err := readAnswer(r, out)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "", "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}

// readAnswer reads one trimmed line. Any read error, including end of input
// after partial text, ends the dialogue.
func readAnswer(r *bufio.Reader, out io.Writer) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		fmt.Fprint(out, "\n")
		return "", errNoAnswer
	}
	return strings.TrimSpace(line), nil
}

// resolveAnswer turns an answer into an absolute folder; ok is false for
// "~user" style answers.
func resolveAnswer(answer, home string) (string, bool) {
	switch {
	case answer == "":
		answer = "~/notes"
	case answer == "~":
		return home, true
	}
	if strings.HasPrefix(answer, "~/") {
		abs, err := userconfig.ExpandGraph(answer, home)
		return abs, err == nil
	}
	if strings.HasPrefix(answer, "~") {
		return "", false
	}
	if filepath.IsAbs(answer) {
		return filepath.Clean(answer), true
	}
	abs, err := filepath.Abs(answer)
	return abs, err == nil
}
