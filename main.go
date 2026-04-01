package main

import (
	"os"

	"github.com/maaslalani/sheets/internal/sheets"
)

func main() {
	os.Exit(sheets.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
