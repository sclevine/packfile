package packfile

import (
	"github.com/sclevine/packfile/layer"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"
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
		mux = mux.For(lp.Name)
		go detectLayer(lp, mux, appDir)
	}
	mux.StreamAll(os.Stdout, os.Stderr)
	mux.WaitAll()

	return nil
}

func detectLayer(l *Layer, mux layer.Mux, appDir string) {
	env := os.Environ()
	env = append(env, "APP="+appDir)

	for _, r := range l.Detect.Require {
		result, ok := mux.Wait(r.Name)
		if !ok {
			mux.Done(layer.Result{Err: xerrors.Errorf("require '%s' not found", r.Name)})
			return
		}
		if result.Err != nil {
			mux.Done(layer.Result{Err: xerrors.Errorf("require '%s' failed: %w", r.Name, result.Err)})
			return
		}
		if r.VersionEnv != "" {
			if version, err := ioutil.ReadFile(filepath.Join(result.Path, "version")); err == nil {
				env = append(env, r.VersionEnv+"="+string(version))
			} else if !os.IsNotExist(err) {
				mux.Done(layer.Result{Err: err})
				return
			}
		}
		if r.MetadataEnv != "" {
			env = append(env, r.MetadataEnv+"="+result.Path)
		}
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