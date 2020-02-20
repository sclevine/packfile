package main

import (
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
	pfPath := filepath.Join(filepath.Dir(filepath.Dir(command)), "packfile.toml")
	if _, err := os.Stat(pfPath); os.IsNotExist(err) {
		pfPath = filepath.Join(".", "packfile.toml")
	}
	if _, err := toml.DecodeFile(pfPath, &pf); err != nil {
		log.Fatalf("Error: %s", err)
	}
	switch filepath.Base(command) {
	case "detect":
		if len(os.Args) != 3 {
			log.Fatal("Error: detect requires two arguments")
		}
		if err := cnb.Detect(&pf, os.Args[1], os.Args[2]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	case "build":
		if len(os.Args) != 4 {
			log.Fatal("Error: build requires three arguments")
		}
		if err := cnb.Build(&pf, os.Args[1], os.Args[2], os.Args[3]); err != nil {
			log.Fatalf("Error: %s", err)
		}
	default:
		log.Fatal("Error: command name must be build or detect")
	}
}
