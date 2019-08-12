package main

import (
	"github.com/sclevine/packfile"
	"log"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func main() {
	if len(os.Args) == 0 {
		log.Fatal("Error: command name missing")
	}
	command := os.Args[0]

	var pf packfile.Packfile
	if _, err := toml.DecodeFile("packfile.toml", &pf); err != nil {
		log.Fatalf("Error: %s", err)
	}
	switch filepath.Base(command) {
	case "detect":
		if len(os.Args) != 3 {
			log.Fatal("Error: detect requires two arguments")
		}
		if err := packfile.Detect(&pf, os.Args[1], os.Args[2]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	case "build":
		if len(os.Args) != 4 {
			log.Fatal("Error: build requires three arguments")
		}
		if err := pf.Build(os.Args[1], os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	default:
		log.Fatal("Error: command name must be build or detect")
	}
}

