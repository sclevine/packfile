package packfile

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile/layer"
)

func Detect(pf *Packfile, platformDir, planPath string) error {
	if err := loadEnv(filepath.Join(platformDir, "env")); err != nil {
		return err
	}
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	var mux layer.Mux
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		mux = mux.For(lp.Name, detectRequires(lp))
		go detectLayer(lp, mux, appDir)
	}
	mux.StreamAll(os.Stdout, os.Stderr)
	mux.WaitAll()

	return nil
}

func detectRequires(l *Layer) []layer.Require {
	var out []layer.Require
	for _, r := range l.Detect.Require {
		out = append(out, layer.Require{
			Name:        r.Name,
			VersionEnv:  r.VersionEnv,
			MetadataEnv: r.MetadataEnv,
		})
	}
	return out
}

func detectLayer(l *Layer, mux layer.Mux, appDir string) {
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

	dir, err := ioutil.TempDir("", "packfile."+l.Name)
	if err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}
	env = append(env, "MD="+dir)
	cmd := exec.Command(l.Detect.Path)
	cmd.Dir = appDir
	cmd.Env = env
	cmd.Stdout = mux.Out()
	cmd.Stderr = mux.Err()
	if err := cmd.Run(); err != nil {
		mux.Done(layer.Result{Err: err})
		return
	}

	mux.Done(layer.Result{Path: dir})
}

func loadEnv(dir string) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		if !fi.IsDir() {
			v, err := ioutil.ReadFile(filepath.Join(dir, fi.Name()))
			if err != nil {
				return err
			}
			if err := os.Setenv(fi.Name(), string(v)); err != nil {
				return err
			}
		}
	}
	return nil
}
