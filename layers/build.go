package layers

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle"
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
	env, err = setupEnvs(env, l.provide().Env, l.LayerDir, l.AppDir)
	if err != nil {
		l.Err = err
		return
	}
	if err := setupProfile(l.provide().Profile, l.LayerDir); err != nil {
		l.Err = err
		return
	}

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
	metadata, err := l.Metadata.ReadAll()
	if err != nil {
		l.Err = err
		return
	}
	versionStr := "."
	if v, ok := metadata["version"].(string); ok {
		versionStr = " with version: " + v
	}
	fmt.Fprintf(w, "Built layer '%s'%s\n", l.Layer.Name, versionStr)
	delete(metadata, "launch")
	delete(metadata, "build")
	layerTOML.Metadata.Saved = metadata
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

func lifecycleEnv(env []string) *lifecycle.Env {
	return &lifecycle.Env{
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
		Setenv: func(key, value string) error {
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
		Environ: func() []string {
			return env
		},
		Map: lifecycle.POSIXBuildEnv,
	}
}

func setupLinkEnv(env []string, links []linkInfo) ([]string, error) {
	lcEnv := lifecycleEnv(env)
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
	return env, nil
}
