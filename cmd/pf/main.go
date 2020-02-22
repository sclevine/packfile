package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/cnb"
)

func main() {
	if len(os.Args) == 0 {
		log.Fatal("Error: command name missing")
	}
	command := os.Args[0]

	var pf packfile.Packfile
	ctxDir := filepath.Dir(filepath.Dir(command))
	if _, err := os.Stat(filepath.Join(ctxDir, "packfile.toml")); os.IsNotExist(err) {
		ctxDir = "."
	}
	if _, err := toml.DecodeFile(filepath.Join(ctxDir, "packfile.toml"), &pf); err != nil {
		log.Fatalf("Error: %s", err)
	}
	switch filepath.Base(command) {
	case "detect":
		if len(os.Args) != 3 {
			log.Fatal("Error: detect requires two arguments")
		}
		if err := cnb.Detect(&pf, ctxDir, os.Args[1], os.Args[2]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	case "build":
		if len(os.Args) != 4 {
			log.Fatal("Error: build requires three arguments")
		}
		if err := cnb.Build(&pf, ctxDir, os.Args[1], os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	default:
		var out, packfile string
		flag.StringVar(&out, "o", "", "output path for buildpack tgz")
		flag.StringVar(&packfile, "f", "", "path to packfile")
		flag.Parse()
		if out == "" {
			log.Fatal("Error: -o must be specified")
		}

	}
}
