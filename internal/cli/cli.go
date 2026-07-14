// Package cli parses arguments, wires the pipeline together, and owns
// the exit-code contract: 0 ok, 1 policy breach, 2 usage error,
// 3 runtime error.
package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/JaydenCJ/eolvet/internal/eoldata"
	"github.com/JaydenCJ/eolvet/internal/version"
)

// Exit codes — stable, documented in the README, relied on by CI gates.
const (
	exitOK      = 0
	exitBreach  = 1
	exitUsage   = 2
	exitRuntime = 3
)

const usage = `eolvet — offline end-of-life audit for repos and Dockerfiles

Usage:
  eolvet scan [flags] [path]          scan a repository or file (default command)
  eolvet check <product> <version>    look up one product version
  eolvet products                     list the bundled EOL snapshot
  eolvet version                      print version and snapshot date

Scan flags:
  --format FMT        text | json | markdown        (default text)
  --as-of DATE        judge as of YYYY-MM-DD        (default today, UTC)
  --warn-within N     eol-soon window in days       (default 90)
  --fail-on WHEN      eol | eol-soon | none         (default eol)
  --strict            unknown findings also fail
  --exclude GLOB      skip matching paths, ** allowed (repeatable)
  --data FILE         use a custom snapshot instead of the bundled one
  --max-file-size N   skip files larger than N bytes (default 1048576)

check also accepts --as-of, --warn-within, --fail-on, and --data;
products accepts --format (text | json) and --data.

Exit codes: 0 ok · 1 policy breach · 2 usage error · 3 runtime error
`

// Run executes the CLI and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runScan(nil, stdout, stderr)
	}
	switch args[0] {
	case "scan":
		return runScan(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "products":
		return runProducts(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		return runVersion(stdout, stderr)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	return runScan(args, stdout, stderr) // a bare flag or path implies scan
}

func runVersion(stdout, stderr io.Writer) int {
	snap, err := eoldata.Load()
	if err != nil {
		fmt.Fprintf(stderr, "eolvet: %v\n", err)
		return exitRuntime
	}
	fmt.Fprintf(stdout, "eolvet %s (snapshot %s)\n", version.Version, snap.SnapshotDate)
	return exitOK
}

// multiFlag collects a repeatable string flag.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// newFlagSet builds a quiet FlagSet whose errors we report ourselves,
// keeping the usage-error exit code under our control.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	return fs
}

// parseInterleaved parses a FlagSet while allowing flags and positional
// arguments in any order ("eolvet check python 3.8 --as-of 2026-01-01"
// must work), returning the positionals in their original order.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// loadSnapshot returns the user's --data snapshot or the bundled one.
func loadSnapshot(dataPath string) (*eoldata.Snapshot, error) {
	if dataPath != "" {
		return eoldata.LoadFile(dataPath)
	}
	return eoldata.Load()
}

// parseAsOf resolves the --as-of flag; empty means today in UTC,
// truncated to date granularity so runs within a day agree.
func parseAsOf(s string) (time.Time, error) {
	if s == "" {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	t, err := eoldata.ParseDate(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --as-of %q (want YYYY-MM-DD)", s)
	}
	return t, nil
}
