// Package skillsync refreshes the user's globally installed shoplazza Agent
// Skills. It covers the path where npm never runs — scripts/update-skills.js
// covers an actual install. Never installs what the user has not; never fatal.
package skillsync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// EnvSkip disables the refresh entirely when set to "1".
	EnvSkip = "SHOPLAZZA_CLI_SKIP_SKILLS"
	// EnvSource overrides the `skills add` source; a local path works.
	EnvSource = "SHOPLAZZA_CLI_SKILLS_SOURCE"
)

// Mirrors scripts/update-skills.js so both refresh paths behave identically.
const (
	defaultSource  = "Shoplazza/shoplazza-cli"
	namePrefix     = "shoplazza-"
	refreshTimeout = 120 * time.Second
)

// Dir returns the global skills directory. Deliberately not overridable:
// `npx skills add -g` resolves the same location from HOME, so a knob moving
// only our end would report on one directory while refreshing another.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agents", "skills")
}

// Source returns the `skills add` source.
func Source() string {
	if v := os.Getenv(EnvSource); v != "" {
		return v
	}
	return defaultSource
}

// installedNames lists the installed shoplazza-* skills, sorted. Missing dir =
// none installed; unreadable dir = error, so breakage can't pass as an opt-out.
func installedNames() ([]string, error) {
	dir := Dir()
	if dir == "" {
		return nil, errors.New("cannot resolve the home directory")
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, namePrefix) {
			continue
		}
		// Detection is by directory, matching update-skills.js: the skills lock
		// file does not reliably record every install.
		if _, err := os.Stat(filepath.Join(dir, name, "SKILL.md")); err != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Installed reports whether any shoplazza-* skill is installed. Not installed
// is a normal state — they are opt-in; an unreadable directory is an error.
func Installed() (bool, error) {
	names, err := installedNames()
	return len(names) > 0, err
}

// Result describes what Refresh did. Ran=false means EnvSkip suppressed it.
type Result struct {
	Installed bool
	Count     int // skills refreshed
	Ran       bool
	Output    string // npx output, for the caller to print on failure
}

// Refresh re-pulls the already-installed skills, installing nothing new.
func Refresh(ctx context.Context) (Result, error) {
	if os.Getenv(EnvSkip) == "1" {
		return Result{}, nil
	}
	names, err := installedNames()
	if err != nil {
		return Result{Ran: true}, err
	}
	if len(names) == 0 {
		return Result{Ran: true}, nil
	}
	res := Result{Installed: true, Count: len(names), Ran: true}

	npx, err := exec.LookPath("npx")
	if err != nil {
		return res, fmt.Errorf("npx not found on PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	// Same argument vector as update-skills.js: -y auto-confirms (every refresh
	// is a full overwrite), -s limits the pull to the installed set.
	args := append([]string{"-y", "skills", "add", Source(), "-g", "-y", "-s"}, names...)
	out, err := exec.CommandContext(ctx, npx, args...).CombinedOutput()
	res.Output = strings.TrimRight(string(out), "\n")
	if err != nil {
		return res, fmt.Errorf("npx skills add: %w", err)
	}
	return res, nil
}

// ManualCommand is the equivalent command a user can run themselves.
func ManualCommand() string {
	names, _ := installedNames()
	return "npx skills add " + Source() + " -g -y -s " + strings.Join(names, " ")
}
