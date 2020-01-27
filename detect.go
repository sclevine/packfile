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
	var layers []linkLayer
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
		layers = append(layers, &detectLayer{
			streamer: lsync.NewStreamer(),
			linkShare: linkShare{
				mdDir:    mdDir,
			},
			layer:    lp,
			shell:    shell,
			appDir:   appDir,
		})
	}
	syncLayers := toSyncLayers(layers)
	for i := range syncLayers {
		go func(i int) {
			defer layers[i].Close()
			syncLayers[i].Run()
		}(i)
	}
	for i := range layers {
		layers[i].Stream(os.Stdout, os.Stderr)
	}
	for i := range syncLayers {
		syncLayers[i].Wait()
	}
	requires, err := readRequires(layers)
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
		if f.IsDir() {
			continue
		}
		if err := fn(f.Name(), filepath.Join(dir, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

// TODO: consider moving to another package with toSyncLayers
func readRequires(layers []linkLayer) ([]planRequire, error) {
	var requires []planRequire
	for _, layer := range layers {
		info := layer.info()
		if IsFail(info.share.err) {
			continue
		} else if info.share.err != nil {
			return nil, xerrors.Errorf("error for layer '%s': %w", info.name, info.share.err)
		}
		if info.share.mdDir == "" {
			continue
		}
		req, err := readRequire(info.name, info.share.mdDir)
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
	streamer
	linkShare
	layer  *Layer
	shell  string
	appDir string
}

func (l *detectLayer) info() layerInfo {
	return layerInfo{
		name:  l.layer.Name,
		share: &l.linkShare,
	}
}

func (l *detectLayer) link(_ linkInfo) {}

func (l *detectLayer) sync(_ lsync.Link) {}

func (l *detectLayer) Links() (links []lsync.Link, forTest bool) {
	return nil, false
}

func (l *detectLayer) Test() (exists, matched bool) {
	return false, false
}

func (l *detectLayer) Run() {
	if err := writeMetadata(l.mdDir, l.layer.Version, l.layer.Metadata); err != nil {
		l.err = err
		return
	}
	if l.layer.Require == nil {
		return
	}

	env := os.Environ()
	env = append(env, "APP="+l.appDir, "MD="+l.mdDir)
	cmd, c, err := execCmd(&l.layer.Require.Exec, l.shell)
	if err != nil {
		l.err = err
		return
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				l.err = CodeError(status.ExitStatus())
				return
			}
		}
		l.err = err
		return
	}
}

type CodeError int

func (e CodeError) Error() string {
	return fmt.Sprintf("failed with code %d", e)
}
