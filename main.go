// Command ctxlint lints agent instruction files (AGENTS.md and SKILL.md) for
// front-matter correctness and token budget overruns.
package main

import (
	"os"

	"github.com/rsn491/ctxlint/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
