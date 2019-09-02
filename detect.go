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
	var requires []planRequire
	var provides []planProvide
	var mux layer.Mux
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Provide != nil {
			provides = append(provides, planProvide{Name: lp.Name})
		}
		if lp.Require == nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		mux = mux.For(lp.Name, detectRequires(lp.Require.Require))
		go detectLayer(lp, mux, shell, mdDir, appDir)
	}
	mux.StreamAll(os.Stdout, os.Stderr)
	for _, res := range mux.WaitAll() {
		if IsFail(res.Err) {
			continue
		} else if err != nil {
			return xerrors.Errorf("error for layer '%s': %w", res.Name, err)
		}
		req, err := readRequire(res.Name, res.Path)
		if err != nil {
			return xerrors.Errorf("invalid metadata for layer '%s': %w", res.Name, err)
		}
		requires = append(requires, req)
	}
	f, err := os.Create(planPath)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(planSections{requires, provides}); err != nil {
		return err
	}
	return nil
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

func detectRequires(requires []DetectRequire) []layer.Require {
	var out []layer.Require
	for _, r := range requires {
		out = append(out, layer.Require{
			Name:        r.Name,
			VersionEnv:  r.VersionEnv,
			MetadataEnv: r.MetadataEnv,
		})
	}
	return out
}

func detectLayer(l *Layer, mux layer.Mux, shell, mdDir, appDir string) {
	if err := writeDetectMetadata(l, mdDir); err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}

	env := os.Environ()
	env = append(env, "APP="+appDir)

	if err := mux.Wait(func(req layer.Require, res layer.Result) error {
		if res.Err != nil {
			return xerrors.Errorf("require '%s' failed: %w", req.Name, res.Err)
		}
		if req.VersionEnv != "" {
			if version, err := ioutil.ReadFile(filepath.Join(res.Path, "version")); err == nil {
				env = append(env, req.VersionEnv+"="+string(version))
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if req.MetadataEnv != "" {
			env = append(env, req.MetadataEnv+"="+res.Path)
		}
		return nil
	}); err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}

	env = append(env, "MD="+mdDir)
	cmd, c, err := execCmd(&l.Require.Exec, shell)
	if err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}
	defer c.Close()
	cmd.Dir = appDir
	cmd.Env = env
	cmd.Stdout = mux.Out()
	cmd.Stderr = mux.Err()
	if err := cmd.Run(); err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}

	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				mux.Done(layer.Result{Err: DetectError(status.ExitStatus())})
				return
			}
		}
		mux.Done(layer.Result{Err: err})
		return
	}

	mux.Done(layer.Result{Path: mdDir})
}

type DetectError int

func (e DetectError) Error() string {
	return fmt.Sprintf("detect failed with code %d", e)
}

func writeDetectMetadata(l *Layer, path string) error {
	for k, v := range l.Metadata {
		if err := ioutil.WriteFile(filepath.Join(path, k), []byte(v), 0666); err != nil {
			return err
		}
	}
	if l.Version == "" {
		return nil
	}
	return ioutil.WriteFile(filepath.Join(path, "version"), []byte(l.Version), 0666)
}