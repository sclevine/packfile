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
	var list []lsync.Runner
	for i := range pf.Caches {
		list = append(list, &cacheLayer{
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
	lock := lsync.NewLock(len(list))

	list.Run()
	list.Stream(os.Stdout, os.Stderr)
	if err := writeTOML(launchTOML{
		Processes: pf.Processes,
	}, filepath.Join(layersDir, "launch.toml")); err != nil {
		return err
	}
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
	layer    *Layer
	result   linkResult
	links    []buildLink
	shell    string
	mdDir    string
	appDir   string
	layerDir string
	requires []planRequire
}

type buildLink struct {
	name string
	lsync.Link
	*linkResult
}

type linkResult struct {
	layerPath    string
	metadataPath string
	err          error
}

func makeLayers(list []linkRunner) []*lsync.Layer {
	lock := lsync.NewLock(len(list))
	var layers []*lsync.Layer
	for i := range list {
		for j := range list[:i] {
			for _, link := range list[i].linkList() {
				if link.Name == list[j].linkName() {
					list[i].addLink(buildLink{
						link.Name,
						layers[j].Link(true, false, false),
						list[j].linkResult(),
					})
				}
			}
			for _, link := range list[j].linkList() {
				if link.Name == list[i].linkName() &&
					(link.LinkContents || link.LinkVersion) {
					list[i].addLink(buildLink{
						list[j].linkName(),
						layers[j].Link(false, link.LinkContents, link.LinkVersion),
						list[j].linkResult(),
					})
				}
			}
		}
		layers = append(layers, lsync.NewLayer(lock, list[i]))
	}
	return layers
}

type linkRunner interface {
	lsync.Runner
	linkName() string
	linkResult() *linkResult
	linkList() []Link
	addLink(link buildLink)
}

func (l *buildLayer) linkName() string {
	return l.layer.Name
}

func (l *buildLayer) linkResult() *linkResult {
	return &l.result
}

func (l *buildLayer) linkList() []Link {
	return l.provide().Links
}

func (l *buildLayer) addLink(link buildLink) {
	l.links = append(l.links, link)
}

func (l *buildLayer) dothing(list []linkRunner, layers []*lsync.Layer) {
	for j := range list {
		for _, link := range l.provide().Links {
			if link.Name == list[j].linkName() {
				l.links = append(l.links, buildLink{
					link.Name,
					layers[j].Link(true, false, false),
					list[j].linkResult(),
				})
			}
		}
		for _, link := range list[j].linkList() {
			if link.Name == l.linkName() && (link.LinkContents || link.LinkVersion) {
				l.links = append(l.links, buildLink{
					list[j].linkName(),
					layers[j].Link(false, link.LinkContents, link.LinkVersion),
					list[j].linkResult(),
				})
			}
		}
	}
}

func (l *buildLayer) Links() (links []lsync.Link, forTest bool) {
	if l.provide() != nil && l.provide().Test != nil {
		forTest = l.provide().Test.UseLinks
	}
	for _, l := range l.links {
		links = append(links, l.Link)
	}
	return links, forTest
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
			lt, err := readLayerTOML(res.LayerTOML())
			if err != nil {
				return lsync.Result{}, err
			}
			env = append(env, res.VersionEnv+"="+lt.Metadata.Version)
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
	if err := writeTOML(layerTOML, layerTOMLPath); err != nil {
		return lsync.Result{}, err
	}

	for _, res := range results {
		if (res.LinkContents && !res.Preserved) ||
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
			lt, err := readLayerTOML(res.LayerTOML())
			if err != nil {
				return lsync.Result{}, err
			}
			env = append(env, res.VersionEnv+"="+lt.Metadata.Version)
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

func (l *cacheLayer) Links() []Link {
	return nil
}
