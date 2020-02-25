package main

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dustin/go-humanize"

	"github.com/sclevine/packfile"
)

func main() {
	if n := len(os.Args); n != 2 && n != 3 {
		log.Fatal("Usage: get-dep <name> [<version>]")
	}
	name := os.Args[1]
	var version string
	if len(os.Args) == 3 {
		version = os.Args[2]
	}
	var config packfile.ConfigTOML
	if _, err := toml.DecodeFile(os.Getenv("PF_CONFIG_PATH"), &config); err != nil {
		log.Fatalf("Error: %s\n", err)
	}
	var dep packfile.Dep
	for _, d := range config.Deps {
		if d.Name == name && (version == "" || d.Version == version) {
			dep = d
		}
	}
	name = fmt.Sprintf("%s-%s", dep.Name, dep.Version)
	out := filepath.Join(config.BuildpackDir, "deps", name)
	var sha []byte
	if _, err := os.Stat(out); err != nil {
		out = filepath.Join(config.StoreDir, name)
		sha, err = download(dep.URI, out)
		if err != nil {
			log.Fatalf("Error: %s\n", err)
		}
	}
	if sha := string(sha); dep.SHA != "" && dep.SHA != sha {
		log.Fatalf("Error: mismatched SHA (%s != %s)\n", sha, dep.SHA)
	}
	// TODO: write metadata to config.MetadataDir
	fmt.Println(out)
}

type writeCounter struct {
	n, len int64
	name   string
	hash   hash.Hash
}

func (w *writeCounter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	if n, err := w.hash.Write(p); err != nil {
		return n, err
	}
	fmt.Printf("\r%s", strings.Repeat(" ", 50))
	size := "unknown"
	if w.len >= 0 {
		size = humanize.Bytes(uint64(w.len))
	}
	fmt.Printf("\rDownloading %s: %s / %s", w.name, humanize.Bytes(uint64(w.n)), size)
	return len(p), nil
}

func download(uri, filepath string) (sha []byte, err error) {
	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return nil, err
	}
	defer out.Close()

	counter := &writeCounter{
		len:  resp.ContentLength,
		name: path.Base(filepath),
		hash: sha256.New(),
	}
	if _, err := io.Copy(out, io.TeeReader(resp.Body, counter)); err != nil {
		return nil, err
	}
	fmt.Println()
	return counter.hash.Sum(nil), out.Close()
}
