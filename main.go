package main

import (
	"os"

	"github.com/Shoplazza/shoplazza-cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
