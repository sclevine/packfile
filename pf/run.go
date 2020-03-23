package pf

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"

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
	if p, err := getPackfile(ctxDir); err == nil {
		mergePackfiles(pf, p)
	} else if !os.IsNotExist(err) {
		return err
	}
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

func mergePackfiles(dst *packfile.Packfile, src packfile.Packfile) {
	if src.API != "" {
		dst.API = src.API
	}
	if src.Config.ID != "" {
		dst.Config.ID = src.Config.ID
	}
	if src.Config.Version != "" {
		dst.Config.Version = src.Config.Version
	}
	if src.Config.Name != "" {
		dst.Config.Name = src.Config.Name
	}
	if src.Config.Shell != "" {
		dst.Config.Shell = src.Config.Shell
	}
	if len(src.Processes) > 0 {
		dst.Processes = append(src.Processes, dst.Processes...)
	}
	if len(src.Caches) > 0 {
		dst.Caches = append(src.Caches, dst.Caches...)
	}
	if len(src.Layers) > 0 {
		dst.Layers = append(src.Layers, dst.Layers...)
	}
	if len(src.Slices) > 0 {
		dst.Slices = append(src.Slices, dst.Slices...)
	}
	if len(src.Stacks) > 0 {
		dst.Stacks = append(src.Stacks, dst.Stacks...)
	}
}

func getPackfile(dir string) (pf packfile.Packfile, err error) {
	if _, err = toml.DecodeFile(filepath.Join(dir, "packfile.toml"), &pf); os.IsNotExist(err) {
		if err = yamlDecode(filepath.Join(dir, "packfile.yaml"), &pf); os.IsNotExist(err) {
			err = os.ErrNotExist
		}
	}
	return
}

func yamlDecode(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return yaml.NewDecoder(f).Decode(v)
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
