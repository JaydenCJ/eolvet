// Command eolvet scans repositories and Dockerfiles for end-of-life
// runtimes, distros, and base images against a bundled, versioned EOL
// data snapshot — fully offline.
package main

import (
	"os"

	"github.com/JaydenCJ/eolvet/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
