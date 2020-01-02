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

type layerTOML struct {
	Launch   bool `toml:"launch"`
	Build    bool `toml:"build"`
	Cache    bool `toml:"cache"`
	Metadata struct {
		Version string `toml:"version"`
	} `toml:"metadata"`
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

func readLayerTOML(path string) (layerTOML, error) {
	var out layerTOML
	if _, err := toml.DecodeFile(path, &out); err != nil {
		if !xerrors.Is(err, os.ErrNotExist) {
			return layerTOML{}, err
		}
		out = layerTOML{}
	}
	return out, nil
}

func writeLayerTOML(path string, lt layerTOML) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(lt)
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
	for i := range pf.Caches {
		list = list.Add(&cacheLayer{
			Streamer: lsync.NewStreamer(),
			cache:    &pf.Caches[i],
			shell:    shell,
			appDir:   appDir,
			cacheDir: filepath.Join(layersDir, pf.Caches[i].Name),
		})
	}
	for i := range pf.Layers {
		lp := &pf.Layers[i]
		if lp.Provide != nil && lp.Build != nil {
			return xerrors.Errorf("layer '%s' has both provide and build sections", lp.Name)
		}
		if lp.Build == nil && lp.Provide == nil && lp.Require != nil {
			continue
		}
		// FIXME: move metadata dir into individual layer Init/Cleanup methods
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		layerDir := filepath.Join(layersDir, lp.Name)
		list = list.Add(&buildLayer{
			Streamer: lsync.NewStreamer(),
			layer:    lp,
			shell:    shell,
			mdDir:    mdDir,
			appDir:   appDir,
			layerDir: layerDir,
			requires: plan.get(lp.Name),
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
	return toml.NewEncoder(f).Encode(buildPlan{requires})
}

type buildLayer struct {
	*lsync.Streamer
	layer       *Layer
	shell       string
	mdDir       string
	appDir      string
	layerDir    string
	requires    []planRequire
}

func (l *buildLayer) Name() string {
	return l.layer.Name
}

func (l *buildLayer) Links() []lsync.Link {
	return l.provide().Links
}

func (l *buildLayer) provide() *Provide {
	provide := l.layer.Build
	if l.layer.Provide != nil {
		provide = l.layer.Provide
	}
	return provide
}

func (l *buildLayer) Test(results []lsync.LinkResult) (lsync.Result, error) {
	if l.provide() != nil {
		if l.layer.Require == nil {
			if err := writeMetadata(l.mdDir, l.layer.Version, l.layer.Metadata); err != nil {
				return lsync.Result{}, err
			}
		}
		for _, req := range l.requires {
			if err := writeMetadata(l.mdDir, req.Version, req.Metadata); err != nil {
				return lsync.Result{}, err
			}
		}
	}

	env := os.Environ()
	env = append(env, "APP="+l.appDir, "MD="+l.mdDir)

	for _, res := range results {
		if res.ForTest && res.PathEnv != "" {
			env = append(env, res.PathEnv+"="+res.LayerPath)
		}
		if res.VersionEnv != "" {
			// FIXME: make VERSION special (always the test version, even after ForTest), by reading TOML(?)
			if version, err := ioutil.ReadFile(filepath.Join(res.MetadataPath, "version")); err == nil {
				env = append(env, res.VersionEnv+"="+string(version))
			} else if !os.IsNotExist(err) {
				return lsync.Result{}, err
			}
		}
		if res.MetadataEnv != "" {
			env = append(env, res.MetadataEnv+"="+res.MetadataPath)
		}
	}
	if l.provide() == nil || l.provide().Test == nil {
		return lsync.Result{
			MetadataPath: l.mdDir,
		}, nil
	}

	cmd, c, err := execCmd(&l.provide().Test.Exec, l.shell)
	if err != nil {
		return lsync.Result{}, err
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return lsync.Result{}, CodeError(status.ExitStatus())
			}
		}
		return lsync.Result{}, err
	}

	layerTOMLPath := l.layerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		return lsync.Result{}, err
	}
	layerTOML.Cache = l.layer.Store
	layerTOML.Build = l.layer.Expose
	layerTOML.Launch = l.layer.Export

	skipVersion := false
	oldVersion := layerTOML.Metadata.Version
	newVersion, err := l.mdValue("version")
	if err == nil {
		layerTOML.Metadata.Version = newVersion
	} else if os.IsNotExist(err) {
		layerTOML.Metadata.Version = ""
		skipVersion = true
	} else {
		return lsync.Result{}, err
	}
	if err := writeLayerTOML(layerTOMLPath, layerTOML); err != nil {
		return lsync.Result{}, err
	}

	for _, res := range results {
		if (res.LinkContents && !res.NoChange) ||
			(res.LinkVersion && !res.SameVersion) {
			if err := os.RemoveAll(l.layerDir); err != nil {
				return lsync.Result{}, err
			}
			return lsync.Result{
				MetadataPath: l.mdDir,
			}, nil
		}
	}

	if !skipVersion && newVersion == oldVersion {
		if _, err := os.Stat(l.layerDir); xerrors.Is(err, os.ErrNotExist) {
			if l.layer.Expose || l.layer.Store {
				return lsync.Result{
					MetadataPath: l.mdDir,
				}, nil
			}
			return lsync.Result{
				MetadataPath: l.mdDir,
			}, layer.ErrNotNeeded
		}
		return lsync.Result{
			LayerPath:    l.layerDir,
			MetadataPath: l.mdDir,
		}, layer.ErrExists
	}
	if err := os.RemoveAll(l.layerDir); err != nil {
		return lsync.Result{}, err
	}
	return lsync.Result{
		MetadataPath: l.mdDir,
	}, nil
}

func (l *buildLayer) mdValue(key string) (string, error) {
	// FIXME: need to account for empty version file matching missing layer TOML
	value, err := ioutil.ReadFile(filepath.Join(l.mdDir, key))
	if err != nil {
		return "", err
	}
	return string(value), err
}

func (l *buildLayer) Run(results []lsync.LinkResult) (lsync.Result, error) {
	env := os.Environ()
	env = append(env, "APP="+l.appDir, "MD="+l.mdDir, "LAYER="+l.layerDir)

	for _, res := range results {
		if res.PathEnv != "" {
			env = append(env, res.PathEnv+"="+res.LayerPath)
		}
		if res.VersionEnv != "" {
			if version, err := ioutil.ReadFile(filepath.Join(res.MetadataPath, "version")); err == nil {
				env = append(env, res.VersionEnv+"="+string(version))
			} else if !os.IsNotExist(err) {
				return lsync.Result{}, err
			}
		}
		if res.MetadataEnv != "" {
			env = append(env, res.MetadataEnv+"="+res.MetadataPath)
		}
	}

	if err := os.MkdirAll(l.layerDir, 0777); err != nil {
		return lsync.Result{}, err
	}
	if l.provide() == nil {
		return lsync.Result{
			LayerPath:    l.layerDir,
			MetadataPath: l.mdDir,
		}, nil
	}
	cmd, c, err := execCmd(&l.provide().Exec, l.shell)
	if err != nil {
		return lsync.Result{}, err
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return lsync.Result{}, CodeError(status.ExitStatus())
			}
		}
		return lsync.Result{}, err
	}

	return lsync.Result{
		LayerPath:    l.layerDir,
		MetadataPath: l.mdDir,
	}, nil
}

type cacheLayer struct {
	*lsync.Streamer
	cache    *Cache
	shell    string
	appDir   string
	cacheDir string
}

func (l *cacheLayer) Run(_ []lsync.LinkResult) (lsync.Result, error) {
	env := os.Environ()
	env = append(env, "APP="+l.appDir, "CACHE="+l.cacheDir)

	if err := os.MkdirAll(l.cacheDir, 0777); err != nil {
		return lsync.Result{}, err
	}
	if l.cache.Setup == nil {
		return lsync.Result{
			LayerPath: l.cacheDir,
		}, nil
	}
	cmd, c, err := execCmd(l.cache.Setup, l.shell)
	if err != nil {
		return lsync.Result{}, err
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return lsync.Result{}, CodeError(status.ExitStatus())
			}
		}
		return lsync.Result{}, err
	}

	return lsync.Result{
		LayerPath: l.cacheDir,
	}, nil
}

func (l *cacheLayer) Name() string {
	return l.cache.Name
}

func (l *cacheLayer) Links() []lsync.Link {
	return nil
}
