package main

import (
	"fmt"
	"os"

	"github.com/selyafi/diffsmith/internal/app"
)

// version is the release tag string. Overridden by the build via
// `-ldflags "-X main.version=v0.1.0"` (see Makefile and the release
// workflow). The literal "dev" indicates an unstamped local build.
var version = "dev"

func main() {
	app.SetVersion(version)
	if err := app.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
