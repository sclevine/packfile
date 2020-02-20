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
	"github.com/sclevine/packfile/sync"
)

type Build struct {
	Streamer
	LinkShare
	Layer       *packfile.Layer
	Requires    []Require
	Shell       string
	AppDir      string
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

func mergeRequire(path string, req Require) error {
	md := copyMap(req.Metadata)
	md["launch"] = mergeBoolStrings(md["launch"], readMetadata(path, "launch"))
	md["build"] = mergeBoolStrings(md["build"], readMetadata(path, "build"))
	md["version"] = req.Version
	return writeAllMetadata(path, md)
}

func mergeBoolStrings(s1, s2 string) string {
	return fmt.Sprintf("%t", s1 == "true" || s2 == "true")
}

func (l *Build) Test() (exists, matched bool) {
	if l.Layer.Require == nil {
		if err := writeLayerMetadata(l.MetadataDir, l.Layer); err != nil {
			l.Err = err
			return false, false
		}
	}
	for _, req := range l.Requires {
		if err := mergeRequire(l.MetadataDir, req); err != nil {
			l.Err = err
			return false, false
		}
	}

	env := os.Environ()
	env = append(env, "APP="+l.AppDir, "MD="+l.MetadataDir)

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
			env = append(env, link.MetadataEnv+"="+link.MetadataDir)
		}
	}
	if l.fullEnv() {
		var err error
		env, err = setupEnv(env, l.links)
		if err != nil {
			l.Err = err
			return
		}
	}
	if l.provide().Test != nil {
		cmd, c, err := execCmd(&l.provide().Test.Exec, l.Shell)
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
	layerTOML.Build = readMetadata(l.MetadataDir, "build") == "true"
	layerTOML.Launch = readMetadata(l.MetadataDir, "launch") == "true"

	// TODO: use cached build ID when store.toml is implemented in lifecycle
	cachedBuildID := l.LastBuildID // layerTOML.Metadata.BuildID
	layerTOML.Metadata.BuildID = l.BuildID

	oldVersion := layerTOML.Metadata.Version
	newVersion := readMetadata(l.MetadataDir, "version")
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

func (l *Build) Run() {
	if l.Err != nil {
		return
	}
	w, _ := l.Streamer.Writers()
	fmt.Fprintf(w, "Building layer %s.\n", l.Layer.Name)
	if err := os.RemoveAll(l.LayerDir); err != nil {
		l.Err = err
		return
	}
	env := os.Environ()
	env = append(env, "APP="+l.AppDir, "MD="+l.MetadataDir, "LAYER="+l.LayerDir)

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
			env = append(env, link.MetadataEnv+"="+link.MetadataDir)
		}
	}
	var err error
	env, err = setupEnv(env, l.links)
	if err != nil {
		l.Err = err
		return
	}

	if err := os.MkdirAll(l.LayerDir, 0777); err != nil {
		l.Err = err
		return
	}
	cmd, c, err := execCmd(&l.provide().Exec, l.Shell)
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
	layerTOML.Metadata.Saved, err = readAllMetadata(l.MetadataDir)
	delete(layerTOML.Metadata.Saved, "launch")
	delete(layerTOML.Metadata.Saved, "build")
	if err != nil {
		l.Err = err
		return
	}
	l.Err = writeTOML(layerTOML, layerTOMLPath)
}

func (l *Build) Skip() {
	if l.Err != nil {
		return
	}
	w, _ := l.Streamer.Writers()
	fmt.Fprintf(w, "Skipping layer %s.\n", l.Layer.Name)

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		l.Err = err
		return
	}
	if err := deleteAllMetadata(l.MetadataDir); err != nil {
		l.Err = err
		return
	}
	saved := copyMap(layerTOML.Metadata.Saved)
	if layerTOML.Launch {
		saved["launch"] = "true"
	}
	if layerTOML.Build {
		saved["build"] = "true"
	}
	l.Err = writeAllMetadata(l.MetadataDir, saved)
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
		writeField(hash, env.Name, env.Value)
	}
	for _, env := range l.provide().Env.Build {
		writeField(hash, env.Name, env.Value)
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
		Version    string            `toml:"version,omitempty"`
		BuildID    string            `toml:"build-id,omitempty"`
		CodeDigest string            `toml:"code-digest"`
		Saved      map[string]string `toml:"saved,omitempty"` // TODO: fails to accept all metadata at build
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
