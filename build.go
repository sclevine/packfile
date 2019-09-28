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
	"github.com/sclevine/packfile/lsync"
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
	list := layer.NewList()
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Provide != nil && lp.Build != nil {
			return xerrors.Errorf("layer '%s' has both provide and build sections", lp.Name)
		}
		//provide := lp.Provide
		//if lp.Build != nil {
		//	provide = lp.Build
		//}
		if lp.Build == nil && lp.Provide == nil && lp.Require != nil {
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
		list = list.Add(&buildLayer{
			Streamer: lsync.NewStreamer(),
			layer: lp,
			shell: shell,
			mdDir: mdDir,
			appDir: appDir,
			layerDir: layerDir,
		})
	}
	list.Run()
	list.Stream(os.Stdout, os.Stderr)
	for _, res := range list.Wait() {
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

type buildLayer struct {
	*lsync.Streamer
	layer    *Layer
	shell    string
	mdDir    string
	appDir   string
	layerDir string
}

func (b *buildLayer) Name() string {
	return b.layer.Name
}

func (b *buildLayer) Links() []lsync.Link {
	build := b.layer.Build
	if b.layer.Provide != nil {
		build = b.layer.Provide
	}
	return build.Links
}

func (b *buildLayer) Test(results []lsync.LinkResult) (lsync.Result, error) {

}

func (b *buildLayer) Run(results []lsync.LinkResult) (lsync.Result, error) {

}

func buildLayer2(l *Layer, requires []planRequire, shell, mdDir, appDir, layerDir string, used bool) {
	if lp.Provide != nil {
		if lp.Require == nil {
			if err := writeMetadata(mdDir, lp.Version, lp.Metadata); err != nil {
				mux.Done(lsync.Result{Err: err})
				return
			}
		}
		for _, req := range requires {
			if err := writeMetadata(mdDir, req.Version, req.Metadata); err != nil {
				mux.Done(lsync.Result{Err: err})
				return
			}
		}
	}

	env := os.Environ()
	env = append(env, "APP="+appDir)

	if err := mux.Wait(func(link lsync.Link, res lsync.Result) error {
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
		mux.Done(lsync.Result{Err: err})
		return
	}

	env = append(env, "MD="+mdDir)
	cmd, c, err := execCmd(&lp.Provide.Test, shell)
	if err != nil {
		mux.Done(lsync.Result{Err: err})
		return
	}
	defer c.Close()
	cmd.Dir = appDir
	cmd.Env = env
	cmd.Stdout = mux.Out()
	cmd.Stderr = mux.Err()
	if err := cmd.Run(); err != nil {
		mux.Done(lsync.Result{Err: err})
		return
	}

	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				mux.Done(lsync.Result{Err: DetectError(status.ExitStatus())})
				return
			}
		}
		mux.Done(lsync.Result{Err: err})
		return
	}

	// compare versions, delete layer or skip

	mux.Done(lsync.Result{LayerPath: layerDir, MetadataPath: mdDir})
}
