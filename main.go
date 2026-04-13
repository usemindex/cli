package main

import "github.com/usemindex/cli/cmd"

// version é injetada pelo goreleaser via ldflags: -X main.version={{.Version}}
var version = "dev"

func main() {
	cmd.Version = version
	cmd.Execute()
}
