package layers

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/BurntSushi/toml"
	lcenv "github.com/buildpacks/lifecycle/env"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type Build struct {
	Streamer
	LinkShare
	Layer       *packfile.Layer
	Requires    []Require
	Shell       string
	AppDir      string
	CtxDir      string
	BuildID     string
	LastBuildID string
	links       []linkInfo
	syncs       []sync.Link
}

func (l *Build) info() linkerInfo {
	return linkerInfo{
		name:  l.Layer.Name,
		share: &l.LinkShare,
		links: l.provide().Links,
		app:   l.provide().WriteApp,
	}
}

func (l *Build) locks(_ linker) bool {
	return false
}

func (l *Build) backward(targets []linker, syncs []*sync.Layer) {
	from := l.info()
	for i := range targets {
		to := targets[i].info()

		for _, link := range from.links {
			if link.Name == to.name {
				l.links = append(l.links, linkInfo{link, to.share})
				l.syncs = append(l.syncs, syncs[i].Link(sync.LinkRequire))
			}
		}

		if targets[i].locks(l) {
			for j := range targets[i+1:] {
				if k := i + 1 + j; targets[i].locks(targets[k]) {
					l.syncs = append(l.syncs, syncs[k].Link(sync.LinkSerial))
				}
			}
		}

		if from.app && to.app {
			l.syncs = append(l.syncs, syncs[i].Link(sync.LinkSerial))
		}
	}
}

func (l *Build) forward(targets []linker, syncs []*sync.Layer) {
	from := l.info()
	for i := range targets {
		to := targets[i].info()

		for _, link := range to.links {
			if link.Name == from.name {
				t := sync.LinkNone
				if link.LinkVersion {
					t = sync.LinkVersion
				}
				if link.LinkContent {
					t = sync.LinkContent
				}
				if t != sync.LinkNone {
					l.syncs = append(l.syncs, syncs[i].Link(t))
				}
			}
		}
	}
}

func (l *Build) Links() (links []sync.Link, forTest bool) {
	return l.syncs, l.fullEnv()
}

func (l *Build) fullEnv() bool {
	if l.provide().Test != nil {
		return l.provide().Test.FullEnv
	}
	return false
}

func (l *Build) provide() *packfile.Provide {
	if l.Layer.Provide != nil {
		return l.Layer.Provide
	}
	return l.Layer.Build
}

func (l *Build) layerTOML() string {
	return l.LayerDir + ".toml"
}

func mergeRequire(store metadata.Store, req Require) error {
	prevLaunch, err := store.Read("launch")
	if err != nil {
		prevLaunch = "false"
	}
	prevBuild, err := store.Read("build")
	if err != nil {
		prevBuild = "false"
	}
	if err := store.DeleteAll(); err != nil {
		return err
	}
	if err := store.WriteAll(req.Metadata); err != nil {
		return err
	}
	nextLaunch, err := store.Read("launch")
	if err != nil {
		nextLaunch = "false"
	}
	nextBuild, err := store.Read("build")
	if err != nil {
		nextBuild = "false"
	}
	others := map[string]interface{}{}
	if req.Version != "" {
		others["version"] = req.Version
	}
	if mergeBoolStrings(nextLaunch, prevLaunch) {
		others["launch"] = "true"
	}
	if mergeBoolStrings(nextBuild, prevBuild) {
		others["build"] = "true"
	}
	return store.WriteAll(others)
}

func mergeBoolStrings(s1, s2 string) bool {
	return s1 == "true" || s2 == "true"
}

func (l *Build) Test() (exists, matched bool) {
	if l.Layer.Require == nil {
		if err := writeLayerMetadata(l.Metadata, l.Layer); err != nil {
			l.Err = err
			return false, false
		}
	}
	for _, req := range l.Requires {
		if err := mergeRequire(l.Metadata, req); err != nil {
			l.Err = err
			return false, false
		}
	}

	env := os.Environ()
	env = append(env, "APP="+l.AppDir, "MD="+l.Metadata.Dir())

	for _, link := range l.links {
		if link.Err != nil {
			l.Err = xerrors.Errorf("link '%s' failed: %w", link.Name, link.Err)
			return false, false
		}
		if l.fullEnv() && link.PathEnv != "" {
			env = append(env, link.PathEnv+"="+link.LayerDir)
		}
		if link.VersionEnv != "" {
			lt, err := readLayerTOML(link.layerTOML())
			if err != nil {
				l.Err = err
				return false, false
			}
			env = append(env, link.VersionEnv+"="+lt.Metadata.Version)
		}
		if link.MetadataEnv != "" {
			env = append(env, link.MetadataEnv+"="+link.Metadata.Dir())
		}
	}
	if l.fullEnv() {
		var err error
		env, err = setupLinkEnv(env, l.links)
		if err != nil {
			l.Err = err
			return
		}
	}
	if l.provide().Test != nil {
		cmd, c, err := execCmd(&l.provide().Test.Exec, l.CtxDir, l.Shell)
		if err != nil {
			l.Err = err
			return false, false
		}
		defer c.Close()
		cmd.Dir = l.AppDir
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = l.Writers()
		if err := cmd.Run(); err != nil {
			if err, ok := err.(*exec.ExitError); ok {
				if status, ok := err.Sys().(syscall.WaitStatus); ok {
					l.Err = CodeError(status.ExitStatus())
					return false, false
				}
			}
			l.Err = err
			return false, false
		}
	}

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		l.Err = err
		return false, false
	}
	layerTOML.Cache = l.Layer.Store
	layerTOML.Build = mdToBool(l.Metadata.Read("build"))
	layerTOML.Launch = mdToBool(l.Metadata.Read("launch"))

	// TODO: use cached build ID when store.toml is implemented in lifecycle
	cachedBuildID := l.LastBuildID // layerTOML.Metadata.BuildID
	layerTOML.Metadata.BuildID = l.BuildID

	oldVersion := layerTOML.Metadata.Version
	newVersion, err := l.Metadata.Read("version")
	if err != nil {
		newVersion = ""
	}
	layerTOML.Metadata.Version = newVersion

	oldDigest := layerTOML.Metadata.CodeDigest
	newDigest := l.digest()
	layerTOML.Metadata.CodeDigest = newDigest

	if err := writeTOML(layerTOML, layerTOMLPath); err != nil {
		l.Err = err
		return false, false
	}

	if cachedBuildID != l.LastBuildID ||
		newDigest != oldDigest ||
		l.provide().WriteApp ||
		l.provide().Test == nil {
		return false, false
	}
	if newVersion != "" && newVersion == oldVersion {
		if _, err := os.Stat(l.LayerDir); xerrors.Is(err, os.ErrNotExist) {
			return false, !l.Layer.Expose && !l.Layer.Store
		}
		return true, true
	}
	return false, false
}

func mdToBool(s string, err error) bool {
	return err == nil && s == "true"
}

func (l *Build) Run() {
	if l.Err != nil {
		return
	}
	w, _ := l.Writers()
	fmt.Fprintf(w, "Building layer '%s'...\n", l.Layer.Name)
	if err := os.RemoveAll(l.LayerDir); err != nil {
		l.Err = err
		return
	}

	env := os.Environ()
	env = append(env, "APP="+l.AppDir, "MD="+l.Metadata.Dir(), "LAYER="+l.LayerDir)

	for _, link := range l.links {
		if link.Err != nil {
			l.Err = xerrors.Errorf("link '%s' failed: %w", link.Name, link.Err)
			return
		}
		if link.PathEnv != "" {
			env = append(env, link.PathEnv+"="+link.LayerDir)
		}
		if link.VersionEnv != "" {
			lt, err := readLayerTOML(link.layerTOML())
			if err != nil {
				l.Err = err
				return
			}
			env = append(env, link.VersionEnv+"="+lt.Metadata.Version)
		}
		if link.MetadataEnv != "" {
			env = append(env, link.MetadataEnv+"="+link.Metadata.Dir())
		}
	}
	var err error
	env, err = setupLinkEnv(env, l.links)
	if err != nil {
		l.Err = err
		return
	}
	if err := os.MkdirAll(l.LayerDir, 0777); err != nil {
		l.Err = err
		return
	}
	env, err = l.setupEnvs(env)
	if err != nil {
		l.Err = err
		return
	}
	if err := l.setupProfile(); err != nil {
		l.Err = err
		return
	}
	env, depsDir, err := l.setupDeps(env)
	if err != nil {
		l.Err = err
		return
	}
	defer os.RemoveAll(depsDir)

	cmd, c, err := execCmd(&l.provide().Exec, l.CtxDir, l.Shell)
	if err != nil {
		l.Err = err
		return
	}
	defer c.Close()
	cmd.Dir = l.AppDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				l.Err = CodeError(status.ExitStatus())
				return
			}
		}
		l.Err = err
		return
	}

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		l.Err = err
		return
	}
	md, err := l.Metadata.ReadAll()
	if err != nil {
		l.Err = err
		return
	}
	versionStr := "."
	if v, ok := md["version"].(string); ok {
		versionStr = " with version: " + v
	}
	fmt.Fprintf(w, "Built layer '%s'%s\n", l.Layer.Name, versionStr)
	delete(md, "launch")
	delete(md, "build")
	layerTOML.Metadata.Saved = md
	l.Err = writeTOML(layerTOML, layerTOMLPath)
}

func (l *Build) Skip() {
	if l.Err != nil {
		return
	}
	w, _ := l.Streamer.Writers()
	fmt.Fprintf(w, "Skipping layer '%s'.\n", l.Layer.Name)

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		l.Err = err
		return
	}
	if err := l.Metadata.DeleteAll(); err != nil {
		l.Err = err
		return
	}
	saved := layerTOML.Metadata.Saved
	if layerTOML.Launch {
		saved["launch"] = "true"
	}
	if layerTOML.Build {
		saved["build"] = "true"
	}
	l.Err = l.Metadata.WriteAll(saved)
}

func (l *Build) digest() string {
	hash := sha256.New()
	writeField(hash, "build")
	writeField(hash, l.provide().Shell, l.provide().Inline)
	writeFile(hash, l.provide().Path)

	for _, dep := range l.provide().Deps {
		writeField(hash, dep.Name, dep.Version, dep.URI, dep.SHA)
	}
	for _, file := range l.provide().Profile {
		writeField(hash, file.Inline)
		writeFile(hash, file.Path)
	}
	for _, env := range l.provide().Env.Launch {
		writeField(hash, env.Name, env.Value, env.Op, env.Delim)
	}
	for _, env := range l.provide().Env.Build {
		writeField(hash, env.Name, env.Value, env.Op, env.Delim)
	}
	for _, link := range l.provide().Links {
		writeField(hash, link.Name, link.PathEnv, link.VersionEnv, link.MetadataEnv)
		fmt.Fprintf(hash, "%t\n%t\n", link.LinkContent, link.LinkVersion)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeField(out io.Writer, values ...string) {
	for _, v := range values {
		fmt.Fprintln(out, v)
	}
}

func writeFile(out io.Writer, path string) {
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintln(out, f)
	}
}

func (l *Build) setupEnvs(env []string) ([]string, error) {
	envs := l.provide().Env
	envBuild := filepath.Join(l.LayerDir, "env.build")
	envLaunch := filepath.Join(l.LayerDir, "env.launch")
	vars := struct {
		Layer string
		App   string
	}{l.LayerDir, l.AppDir}

	if err := setupEnvDir(envs.Build, envBuild, vars); err != nil {
		return nil, err
	}
	lcEnv := lcenv.Env{RootDirMap: lcenv.POSIXBuildEnv, Vars: envToMap(env)}
	if err := lcEnv.AddEnvDir(envBuild); err != nil {
		return nil, err
	}
	return lcEnv.List(), setupEnvDir(envs.Launch, envLaunch, vars)
}

func setupEnvDir(env []packfile.Env, path string, vars interface{}) error {
	if err := os.Mkdir(path, 0777); err != nil {
		return err
	}
	for _, e := range env {
		if e.Name == "" {
			continue
		}
		if e.Op == "" {
			e.Op = "override"
		}
		var err error
		e.Value, err = interpolate(e.Value, vars)
		if err != nil {
			return err
		}
		path := filepath.Join(path, e.Name+"."+e.Op)
		if err := ioutil.WriteFile(path, []byte(e.Value), 0777); err != nil {
			return err
		}
		if e.Delim != "" {
			path := filepath.Join(path, e.Name+".delim")
			if err := ioutil.WriteFile(path, []byte(e.Delim), 0777); err != nil {
				return err
			}
		}
	}
	return nil
}

func interpolate(text string, vars interface{}) (string, error) {
	tmpl, err := template.New("vars").Parse(text)
	if err != nil {
		return "", err
	}
	out := &bytes.Buffer{}
	if err := tmpl.Execute(out, vars); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (l *Build) setupProfile() error {
	profiles := l.provide().Profile
	pad := 1 + int(math.Log10(float64(len(profiles))))
	profiled := filepath.Join(l.LayerDir, "profile.d")
	if err := os.Mkdir(profiled, 0777); err != nil {
		return err
	}
	for i, file := range profiles {
		path := filepath.Join(profiled, fmt.Sprintf("%0*d.sh", pad, i))
		if file.Inline != "" {
			err := ioutil.WriteFile(path, []byte(file.Inline), 0777)
			if err != nil {
				return err
			}
		} else if file.Path != "" {
			if err := copyFileContents(path, file.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFileContents(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func (l *Build) setupDeps(env []string) (envOut []string, dir string, err error) {
	dir, err = ioutil.TempDir("", "packfile.deps."+l.Layer.Name)
	if err != nil {
		return nil, "", err
	}
	storeDir := filepath.Join(dir, "store")
	if err := os.Mkdir(storeDir, 0777); err != nil {
		return nil, "", err
	}
	var deps []packfile.Dep
	vars, err := l.Metadata.ReadAll()
	if err != nil {
		return nil, "", err
	}
	for _, dep := range l.provide().Deps {
		if dep.Name, err = interpolate(dep.Name, vars); err != nil {
			return nil, "", err
		}
		if dep.Version, err = interpolate(dep.Version, vars); err != nil {
			return nil, "", err
		}
		if dep.URI, err = interpolate(dep.URI, vars); err != nil {
			return nil, "", err
		}
		if dep.SHA, err = interpolate(dep.SHA, vars); err != nil {
			return nil, "", err
		}
		deps = append(deps, dep)
	}
	configPath := filepath.Join(dir, "config.toml")
	if err := writeTOML(packfile.ConfigTOML{
		ContextDir:  l.CtxDir,
		StoreDir:    storeDir,
		MetadataDir: l.Metadata.Dir(),
		Deps:        deps,
	}, configPath); err != nil {
		return nil, "", err
	}
	return append(env, "PF_CONFIG_PATH="+configPath), dir, nil
}

type layerTOML struct {
	Launch   bool `toml:"launch"`
	Build    bool `toml:"build"`
	Cache    bool `toml:"cache"`
	Metadata struct {
		Version    string                 `toml:"version,omitempty"`
		BuildID    string                 `toml:"build-id,omitempty"`
		CodeDigest string                 `toml:"code-digest"`
		Saved      map[string]interface{} `toml:"saved,omitempty"`
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

func setupLinkEnv(env []string, links []linkInfo) ([]string, error) {
	lcEnv := lcenv.Env{RootDirMap: lcenv.POSIXBuildEnv, Vars: envToMap(env)}
	for _, link := range links {
		if err := lcEnv.AddRootDir(link.LayerDir); err != nil {
			return nil, err
		}
	}
	for _, link := range links {
		if err := lcEnv.AddEnvDir(filepath.Join(link.LayerDir, "env")); err != nil {
			return nil, err
		}
		if err := lcEnv.AddEnvDir(filepath.Join(link.LayerDir, "env.build")); err != nil {
			return nil, err
		}
	}
	return lcEnv.List(), nil
}

func envToMap(env []string) map[string]string {
	vars := map[string]string{}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		vars[parts[0]] = parts[1]
	}
	return vars
}
