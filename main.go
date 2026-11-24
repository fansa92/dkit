package main

import (
	"os"

	"github.com/fansa92/dkit/internal/cli"
	_ "github.com/fansa92/dkit/internal/tools"
)

// These are overridden by -ldflags "-X main.version=... -X main.commit=...
// -X main.date=..." in the release workflow; a plain `go build` keeps the
// defaults, and `dkit about` says so.
var (
	version = "0.1.1"
	commit  = ""
	date    = ""
)

func main() {
	cli.SetBuildInfo(version, commit, date)
	os.Exit(cli.Run(os.Args[1:]))
}
