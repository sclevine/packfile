package packfile

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/packfile/layer"
	"golang.org/x/xerrors"
)

type buildPlan struct {
	Entries []planRequire `toml:"entries"`
}

func (b buildPlan) get(name string) []planRequire {
	var out []planRequire
	for _, e := range b.Entries {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}

func Build(pf *Packfile, layersDir, platformDir, planPath string) error {
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	shell := defaultShell
	if s := pf.Config.Shell; s != "" {
		shell = s
	}
	var plan buildPlan
	if _, err := toml.DecodeFile(planPath, &plan); err != nil {
		return err
	}
	var mux layer.Mux
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Build == nil && lp.Detect != nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		if lp.Build != nil {
			for _, req := range plan.get(lp.Name) {
				if err := writeBuildMetadata(req, mdDir); err != nil {
					return err
				}
			}
		}
		// compare versions, delete layer or skip
		layerDir := filepath.Join(layersDir, lp.Name)
		if err := os.MkdirAll(layerDir, 0777); err != nil {
			return err
		}
		mux = mux.For(lp.Name, buildRequires(lp.Build.Require))
		go buildLayer(lp, mux, shell, mdDir, appDir, layerDir)
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

func buildRequires(requires []BuildRequire) []layer.Require {
	var out []layer.Require
	for _, r := range requires {
		out = append(out, layer.Require{
			Name:        r.Name,
			Write:       r.Write,
			PathEnv:     r.PathEnv,
			VersionEnv:  r.VersionEnv,
			MetadataEnv: r.MetadataEnv,
		})
	}
	return out
}

func writeBuildMetadata(req planRequire, path string) error {
	for k, v := range req.Metadata {
		if err := ioutil.WriteFile(filepath.Join(path, k), []byte(v), 0666); err != nil {
			return err
		}
	}
	if req.Version == "" {
		return nil
	}
	return ioutil.WriteFile(filepath.Join(path, "version"), []byte(req.Version), 0666)
}

func buildLayer(l *Layer, mux layer.Mux, shell, mdDir, appDir, layerDir string) {
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
	cmd, c, err := execCmd(&l.Detect.Exec, shell)
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
