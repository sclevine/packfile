package packfile

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile/layer"
	"github.com/sclevine/packfile/lsync"
)

type planProvide struct {
	Name string `toml:"name"`
}

type planRequire struct {
	Name     string            `toml:"name"`
	Version  string            `toml:"version"`
	Metadata map[string]string `toml:"metadata"` // TODO: fails to accept all metadata at build
}

type planSections struct {
	Requires []planRequire `toml:"requires"`
	Provides []planProvide `toml:"provides"`
}

func Detect(pf *Packfile, platformDir, planPath string) error {
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	shell := defaultShell
	if s := pf.Config.Shell; s != "" {
		shell = s
	}
	var provides []planProvide
	list := layer.NewList()
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Provide != nil || lp.Build != nil {
			provides = append(provides, planProvide{Name: lp.Name})
		}
		if lp.Require == nil && lp.Build == nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		list = list.Add(&detectLayer{
			Streamer: lsync.NewStreamer(),
			layer:    lp,
			shell:    shell,
			mdDir:    mdDir,
			appDir:   appDir,
		})
	}
	list.Run()
	list.Stream(os.Stdout, os.Stderr)
	requires, err := readRequires(list.Wait())
	if err != nil {
		return err
	}
	f, err := os.Create(planPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(planSections{requires, provides})
}

func eachFile(dir string, fn func(name, path string) error) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if err := fn(f.Name(), filepath.Join(dir, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

func readRequires(layers []linkLayer) ([]planRequire, error) {
	var requires []planRequire
	for _, layer := range layers {
		info := layer.info()
		if IsFail(info.result.err) {
			continue
		} else if info.result.err != nil {
			return nil, xerrors.Errorf("error for layer '%s': %w", info.name, info.result.err)
		}
		req, err := readRequire(info.name, info.result.metadataPath)
		if err != nil {
			return nil, xerrors.Errorf("invalid metadata for layer '%s': %w", info.name, err)
		}
		requires = append(requires, req)
	}
	return requires, nil
}

func readRequire(name, path string) (planRequire, error) {
	out := planRequire{
		Name:     name,
		Metadata: map[string]string{},
	}
	if err := eachFile(path, func(name, path string) error {
		value, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		if name == "version" {
			out.Version = string(value)
		} else {
			out.Metadata[name] = string(value)
		}
		return nil
	}); err != nil {
		return planRequire{}, err
	}
	return out, nil
}

type detectLayer struct {
	*lsync.Streamer
	layer  *Layer
	shell  string
	mdDir  string
	appDir string
}

func (d *detectLayer) Name() string {
	return d.layer.Name
}

func (d *detectLayer) Links() []Link {
	return nil
}

func (d *detectLayer) Run(_ []lsync.LinkResult) (lsync.Result, error) {
	if err := writeMetadata(d.mdDir, d.layer.Version, d.layer.Metadata); err != nil {
		return lsync.Result{}, err
	}
	if d.layer.Require == nil {
		return lsync.Result{MetadataPath: d.mdDir}, nil
	}

	env := os.Environ()
	env = append(env, "APP="+d.appDir, "MD="+d.mdDir)
	cmd, c, err := execCmd(&d.layer.Require.Exec, d.shell)
	if err != nil {
		return lsync.Result{}, err
	}
	defer c.Close()
	cmd.Dir = d.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = d.Streamer.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return lsync.Result{}, CodeError(status.ExitStatus())

			}
		}
		return lsync.Result{}, err
	}

	return lsync.Result{MetadataPath: d.mdDir}, nil
}

type CodeError int

func (e CodeError) Error() string {
	return fmt.Sprintf("failed with code %d", e)
}
