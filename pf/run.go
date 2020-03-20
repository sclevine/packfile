package pf

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/cnb"
	depspkg "github.com/sclevine/packfile/deps"
)

func Run(pf *packfile.Packfile) error {
	if len(os.Args) == 0 {
		return errors.New("command name missing")
	}
	command := os.Args[0]
	ctxDir := filepath.Dir(filepath.Dir(command))
	switch filepath.Base(command) {
	case "detect":
		if len(os.Args) != 3 {
			return errors.New("detect requires two arguments")
		}
		if err := cnb.Detect(pf, ctxDir, os.Args[1], os.Args[2]); err != nil {
			return err
		}
	case "build":
		if len(os.Args) != 4 {
			return errors.New("build requires three arguments")
		}
		if err := cnb.Build(pf, ctxDir, os.Args[1], os.Args[2], os.Args[3]); err != nil {
			return err
		}
	}
	return nil
}

type Downloader struct {
	depsClient
	tmpDir string
}

type depsClient interface {
	Get(name, version string) io.ReadCloser
	GetFile(name, version string) (path string, err error)
}

func NewDownloader(md packfile.Metadata, deps []packfile.Dep) (*Downloader, error) {
	if len(os.Args) == 0 {
		return nil, errors.New("command name missing")
	}
	command := os.Args[0]
	ctxDir := filepath.Dir(filepath.Dir(command))
	tmpDir, err := ioutil.TempDir("", "packfile.deps")
	if err != nil {
		return nil, err
	}
	return &Downloader{&depspkg.Client{
		ContextDir: ctxDir,
		StoreDir:   tmpDir,
		Metadata:   md,
		Deps:       deps,
	}, tmpDir}, nil
}

func (d *Downloader) Close() error {
	if d.tmpDir == "" {
		return nil
	}
	if err := os.RemoveAll(d.tmpDir); err != nil {
		return err
	}
	d.tmpDir = ""
	return nil
}
