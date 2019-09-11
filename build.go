package packfile

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile/layer"
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
		if lp.Provide == nil && lp.Require != nil {
			continue
		}
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		layerDir := filepath.Join(layersDir, lp.Name)
		if err := os.MkdirAll(layerDir, 0777); err != nil {
			return err
		}
		mux = mux.For(lp.Name, lp.Provide.Links...)
		go buildLayer(lp, mux, plan.get(lp.Name), shell, mdDir, appDir, layerDir, isUsed(lp.Name, pf.Layers[i+1:]))
	}
	mux.StreamAll(os.Stdout, os.Stderr)
	for _, res := range mux.WaitAll() {
		if IsFail(res.Err) {
			continue
		} else if err != nil {
			return xerrors.Errorf("error for layer '%s': %w", res.Name, err)
		}
		req, err := readRequire(res.Name, res.MetadataPath)
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

func isUsed(name string, layers []Layer) bool {
	for _, l := range layers {
		for _, link := range l.Provide.Links {
			if link.Name == name {
				return true
			}
		}
	}
	return false
}

func buildLayer(lp *Layer, mux layer.Mux, requires []planRequire, shell, mdDir, appDir, layerDir string, used bool) {
	if lp.Provide != nil {
		if lp.Require == nil {
			if err := writeMetadata(mdDir, lp.Version, lp.Metadata); err != nil {
				mux.Done(layer.Result{Err: err})
				return
			}
		}
		for _, req := range requires {
			if err := writeMetadata(mdDir, req.Version, req.Metadata); err != nil {
				mux.Done(layer.Result{Err: err})
				return
			}
		}
	}

	env := os.Environ()
	env = append(env, "APP="+appDir)

	if err := mux.Wait(func(link layer.Link, res layer.Result) error {
		if res.Err != nil {
			return xerrors.Errorf("failed to link '%s': %w", link.Name, res.Err)
		}
		if link.PathEnv != "" {
			env = append(env, link.PathEnv+"="+res.LayerPath)
		}
		if link.VersionEnv != "" {
			if version, err := ioutil.ReadFile(filepath.Join(res.MetadataPath, "version")); err == nil {
				env = append(env, link.VersionEnv+"="+string(version))
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if link.MetadataEnv != "" {
			env = append(env, link.MetadataEnv+"="+res.MetadataPath)
		}
		return nil
	}); err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}

	env = append(env, "MD="+mdDir)
	cmd, c, err := execCmd(&lp.Provide.Test, shell)
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

	// compare versions, delete layer or skip

	mux.Done(layer.Result{LayerPath: layerDir, MetadataPath: mdDir})
}
