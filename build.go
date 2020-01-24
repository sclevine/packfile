package packfile

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle"
	"golang.org/x/xerrors"

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

type layerTOML struct {
	Launch   bool `toml:"launch"`
	Build    bool `toml:"build"`
	Cache    bool `toml:"cache"`
	Metadata struct {
		Version string `toml:"version"`
	} `toml:"metadata"`
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

type launchTOML struct {
	Processes []Process `toml:"processes"`
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
	var layers []linkLayer
	for i := range pf.Caches {
		layers = append(layers, &cacheLayer{
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
		// TODO: don't allow provide() to return nil, add test() with same idea?
		if lp.Build == nil && lp.Provide == nil && lp.Require != nil {
			continue
		}
		// TODO: move metadata dir into individual layer Init/Cleanup methods
		mdDir, err := ioutil.TempDir("", "packfile."+lp.Name)
		if err != nil {
			return err
		}
		defer os.RemoveAll(mdDir)
		layerDir := filepath.Join(layersDir, lp.Name)
		layers = append(layers, &buildLayer{
			Streamer: lsync.NewStreamer(),
			layer:    lp,
			requires: plan.get(lp.Name),
			shell:    shell,
			mdDir:    mdDir,
			appDir:   appDir,
			layerDir: layerDir,
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
	if err := writeTOML(launchTOML{
		Processes: pf.Processes,
	}, filepath.Join(layersDir, "launch.toml")); err != nil {
		return err
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
	return toml.NewEncoder(f).Encode(buildPlan{requires})
}

type buildLayer struct {
	*lsync.Streamer
	layer    *Layer
	requires []planRequire
	links    []linkInfo
	syncs    []lsync.Link
	result   linkResult
	shell    string
	mdDir    string
	appDir   string
	layerDir string
}

type linkLayer interface {
	lsync.Runner
	Stream(out, err io.Writer)
	Close()
	sync(sync lsync.Link)
	link(link linkInfo)
	info() layerInfo
}

type linkResult struct {
	layerDir string
	mdDir    string
	err      error
}

type linkInfo struct {
	Link
	*linkResult
}

func (l linkInfo) layerTOML() string {
	return l.layerDir + ".toml"
}

type layerInfo struct {
	name   string
	result *linkResult
	links  []Link
}

// TODO: separate package, but only after moving Link to separate package
func toSyncLayers(layers []linkLayer) []*lsync.Layer {
	lock := lsync.NewLock(len(layers))
	var out []*lsync.Layer
	for i := range layers {
		from := layers[i].info()
		for j := range layers[:i] {
			to := layers[j].info()
			for _, link := range from.links {
				if link.Name == to.name {
					layers[i].link(linkInfo{link, to.result})
					layers[i].sync(out[j].Link(true, false, false))
				}
			}
			for _, link := range to.links {
				if link.Name == from.name &&
					(link.LinkContents || link.LinkVersion) {
					layers[i].sync(out[j].Link(false, link.LinkContents, link.LinkVersion))
				}
			}
		}
		out = append(out, lsync.NewLayer(lock, layers[i]))
	}
	return out
}

func (l *buildLayer) info() layerInfo {
	return layerInfo{
		name:   l.layer.Name,
		result: &l.result,
		links:  l.provide().Links,
	}
}

func (l *buildLayer) link(link linkInfo) {
	l.links = append(l.links, link)
}

func (l *buildLayer) sync(sync lsync.Link) {
	l.syncs = append(l.syncs, sync)
}

func (l *buildLayer) Links() (links []lsync.Link, forTest bool) {
	return l.syncs, l.forTest()
}

func (l *buildLayer) forTest() bool {
	if l.provide() != nil && l.provide().Test != nil {
		return l.provide().Test.UseLinks
	}
	return false
}

func (l *buildLayer) provide() *Provide {
	provide := l.layer.Build
	if l.layer.Provide != nil {
		provide = l.layer.Provide
	}
	return provide
}

func (l *buildLayer) layerTOML() string {
	return l.layerDir + ".toml"
}

func (l *buildLayer) Test() (exists, matched bool) {
	if l.provide() != nil {
		if l.layer.Require == nil {
			if err := writeMetadata(l.mdDir, l.layer.Version, l.layer.Metadata); err != nil {
				l.result.err = err
				return false, false
			}
		}
		for _, req := range l.requires {
			if err := writeMetadata(l.mdDir, req.Version, req.Metadata); err != nil {
				l.result.err = err
				return false, false
			}
		}
	}

	env := os.Environ()
	env = append(env, "APP="+l.appDir, "MD="+l.mdDir)

	for _, link := range l.links {
		if link.err != nil {
			l.result.err = xerrors.Errorf("link '%s' failed: %w", link.Name, link.err)
			return false, false
		}
		if l.forTest() && link.PathEnv != "" {
			env = append(env, link.PathEnv+"="+link.layerDir)
		}
		if link.VersionEnv != "" {
			lt, err := readLayerTOML(link.layerTOML())
			if err != nil {
				l.result.err = err
				return false, false
			}
			env = append(env, link.VersionEnv+"="+lt.Metadata.Version)
		}
		if link.MetadataEnv != "" {
			env = append(env, link.MetadataEnv+"="+link.mdDir)
		}
	}
	if l.forTest() {
		var err error
		env, err = setupEnv(env, l.links)
		if err != nil {
			l.result.err = err
			return
		}
	}
	if l.provide() == nil || l.provide().Test == nil {
		l.result.mdDir = l.mdDir
		return false, false
	}

	cmd, c, err := execCmd(&l.provide().Test.Exec, l.shell)
	if err != nil {
		l.result.err = err
		return false, false
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				l.result.err = CodeError(status.ExitStatus())
				return false, false
			}
		}
		l.result.err = err
		return false, false
	}

	layerTOMLPath := l.layerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		l.result.err = err
		return false, false
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
		l.result.err = err
		return false, false
	}
	if err := writeTOML(layerTOML, layerTOMLPath); err != nil {
		l.result.err = err
		return false, false
	}

	if !skipVersion && newVersion == oldVersion {
		if _, err := os.Stat(l.layerDir); xerrors.Is(err, os.ErrNotExist) {
			if l.layer.Expose || l.layer.Store {
				l.result.mdDir = l.mdDir
				return false, false
			}
			l.result.mdDir = l.mdDir
			return false, true
		}
		l.result.layerDir = l.layerDir
		l.result.mdDir = l.mdDir
		return true, true
	}
	l.result.mdDir = l.mdDir
	return false, false
}

func (l *buildLayer) mdValue(key string) (string, error) {
	// FIXME: need to account for empty version file matching missing layer TOML
	value, err := ioutil.ReadFile(filepath.Join(l.mdDir, key))
	if err != nil {
		return "", err
	}
	return string(value), err
}

func (l *buildLayer) Run() {
	if l.result.err != nil {
		return
	}
	if err := os.RemoveAll(l.layerDir); err != nil {
		l.result.err = err
		return
	}
	env := os.Environ()
	env = append(env, "APP="+l.appDir, "MD="+l.mdDir, "LAYER="+l.layerDir)

	for _, link := range l.links {
		if link.err != nil {
			l.result.err = xerrors.Errorf("link '%s' failed: %w", link.Name, link.err)
			return
		}
		if link.PathEnv != "" {
			env = append(env, link.PathEnv+"="+link.layerDir)
		}
		if link.VersionEnv != "" {
			lt, err := readLayerTOML(link.layerTOML())
			if err != nil {
				l.result.err = err
				return
			}
			env = append(env, link.VersionEnv+"="+lt.Metadata.Version)
		}
		if link.MetadataEnv != "" {
			env = append(env, link.MetadataEnv+"="+link.mdDir)
		}
	}
	var err error
	env, err = setupEnv(env, l.links)
	if err != nil {
		l.result.err = err
		return
	}

	if err := os.MkdirAll(l.layerDir, 0777); err != nil {
		l.result.err = err
		return
	}
	if l.provide() == nil {
		l.result.layerDir = l.layerDir
		l.result.mdDir = l.mdDir
		return
	}
	cmd, c, err := execCmd(&l.provide().Exec, l.shell)
	if err != nil {
		l.result.err = err
		return
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				l.result.err = CodeError(status.ExitStatus())
				return
			}
		}
		l.result.err = err
		return
	}
	l.result.layerDir = l.layerDir
	l.result.mdDir = l.mdDir
}

type cacheLayer struct {
	*lsync.Streamer
	cache    *Cache
	syncs    []lsync.Link
	result   linkResult
	shell    string
	appDir   string
	cacheDir string
}

func (l *cacheLayer) info() layerInfo {
	return layerInfo{
		name:   l.cache.Name,
		result: &l.result,
	}
}

func (l *cacheLayer) link(_ linkInfo) {}

func (l *cacheLayer) sync(sync lsync.Link) {
	l.syncs = append(l.syncs, sync)
}

func (l *cacheLayer) Links() (links []lsync.Link, forTest bool) {
	return l.syncs, false
}

func (l *cacheLayer) Test() (exists, matched bool) {
	if err := writeTOML(layerTOML{Cache: true}, l.cacheDir+".toml"); err != nil {
		l.result.err = err
		return false, false
	}
	if _, err := os.Stat(l.cacheDir); xerrors.Is(err, os.ErrNotExist) {
		return false, false
	} else if err != nil {
		l.result.err = err
		return false, false
	}
	return true, true
}

func (l *cacheLayer) Run() {
	if l.result.err != nil {
		return
	}
	env := os.Environ()
	env = append(env, "APP="+l.appDir, "CACHE="+l.cacheDir)

	if err := os.MkdirAll(l.cacheDir, 0777); err != nil {
		l.result.err = err
		return
	}
	if l.cache.Setup == nil {
		l.result.layerDir = l.cacheDir
		return
	}
	cmd, c, err := execCmd(l.cache.Setup, l.shell)
	if err != nil {
		l.result.err = err
		return
	}
	defer c.Close()
	cmd.Dir = l.appDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				l.result.err = CodeError(status.ExitStatus())
				return
			}
		}
		l.result.err = err
		return
	}

	l.result.layerDir = l.cacheDir
	return
}

func setupEnv(env []string, links []linkInfo) ([]string, error) {
	lcEnv := &lifecycle.Env{
		LookupEnv: func(key string) (string, bool) {
			for i := range env {
				kv := strings.SplitN(env[i], "=", 2)
				if len(kv) == 2 && kv[0] == key {
					return kv[1], true
				}
			}
			return "", false
		},
		Getenv: func(key string) string {
			for i := range env {
				kv := strings.SplitN(env[i], "=", 2)
				if len(kv) == 2 && kv[0] == key {
					return kv[1]
				}
			}
			return ""
		},
		Setenv:  func(key, value string) error {
			i := 0
			for _, e := range env {
				kv := strings.SplitN(e, "=", 2)
				if len(kv) == 2 && kv[0] != key {
					env[i] = e
					i++
				}
			}
			env = append(env[:i], key+"="+value)
			return nil
		},
		Unsetenv: func(key string) error {
			i := 0
			for _, e := range env {
				kv := strings.SplitN(e, "=", 2)
				if len(kv) == 2 && kv[0] != key {
					env[i] = e
					i++
				}
			}
			env = env[:i]
			return nil
		},
		Environ:  func() []string {
			return env
		},
		Map:      lifecycle.POSIXBuildEnv,
	}

	for _, link := range links {
		if err := lcEnv.AddRootDir(link.layerDir); err != nil {
			return nil, err
		}
	}
	for _, link := range links {
		if err := lcEnv.AddEnvDir(filepath.Join(link.layerDir, "env")); err != nil {
			return nil, err
		}
		if err := lcEnv.AddEnvDir(filepath.Join(link.layerDir, "env.build")); err != nil {
			return nil, err
		}
	}
	return env, nil
}
