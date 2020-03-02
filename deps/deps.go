package deps

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

func Get(config *packfile.ConfigTOML, name, version string) (path string, err error) {
	var dep packfile.Dep
	for _, d := range config.Deps {
		if d.Name == name && (version == "" || d.Version == version) {
			dep = d
		}
	}
	name = fmt.Sprintf("%s@%s", dep.Name, dep.Version)
	out := filepath.Join(config.ContextDir, "deps", name)
	var sha string
	if _, err := os.Stat(out); err != nil {
		out = filepath.Join(config.StoreDir, name)
		sha, err = download(dep.URI, out)
		if err != nil {
			return "", err
		}
	} else {
		sha, err = checksum(out)
		if err != nil {
			return "", err
		}
	}
	if dep.SHA != "" && dep.SHA != sha {
		return "", xerrors.Errorf("mismatched SHA (%s != %s)\n", sha, dep.SHA)
	}
	md := map[string]interface{}{"name": dep.Name}
	if dep.Version != "" {
		md["version"] = dep.Version
	}
	if dep.URI != "" {
		md["uri"] = dep.URI
	}
	if sha != "" {
		md["sha"] = sha
	}
	if len(dep.Metadata) > 0 {
		md["metadata"] = dep.Metadata
	}
	store := metadata.NewFS(config.MetadataDir)
	if err := store.WriteAll(map[string]interface{}{
		"deps": map[string]interface{}{name: md},
	}); err != nil {
		return "", err
	}
	return out, nil
}

type writeCounter struct {
	n, len int64
	name   string
	hash   hash.Hash
	term   bool
}

func (w *writeCounter) Write(p []byte) (int, error) {
	n, err := w.hash.Write(p)
	w.n += int64(n)
	if w.term {
		fmt.Fprintf(os.Stderr, "\r%s", strings.Repeat(" ", 50))
		size := "unknown"
		if w.len >= 0 {
			size = humanize.Bytes(uint64(w.len))
		}
		fmt.Fprintf(os.Stderr, "\rDownloading %s: %s / %s", w.name, humanize.Bytes(uint64(w.n)), size)
	}
	return n, err
}

func (w *writeCounter) Flush() {
	if w.term {
		fmt.Fprintln(os.Stderr)
	} else {
		fmt.Fprintf(os.Stderr, "Downloaded %s (%s)\n", w.name, humanize.Bytes(uint64(w.n)))
	}
}

func download(uri, filepath string) (sha string, err error) {
	resp, err := http.Get(uri)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	counter := &writeCounter{
		len:  resp.ContentLength,
		name: path.Base(filepath),
		hash: sha256.New(),
		term: terminal.IsTerminal(int(os.Stderr.Fd())),
	}
	tee := sync.NewPTeeReader(resp.Body, counter)
	if _, err := io.Copy(out, tee); err != nil {
		return "", err
	}
	if _, err := tee.Sync(); err != nil {
		return "", err
	}
	counter.Flush()
	return fmt.Sprintf("%x", counter.hash.Sum(nil)), out.Close()
}

func checksum(filepath string) (sha string, err error) {
	f, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
