package layers

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"text/template"

	"github.com/BurntSushi/toml"
	lcenv "github.com/buildpacks/lifecycle/env"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type Build struct {
	link.Streamer
	link.Share
	*sync.Kernel
	Layer         *packfile.Layer
	ProvideRunner packfile.ProvideRunner
	TestRunner    packfile.TestRunner
	Requires      []link.Require
	AppDir        string
	BuildID       string
	LastBuildID   string
	links         []linkInfo
	syncs         []sync.Link
}

func (l *Build) Info() link.Info {
	return link.Info{
		Name:  l.Layer.Name,
		Share: &l.Share,
		Links: l.provide().Links,
		App:   l.provide().LockApp,
	}
}

func (l *Build) Locks(_ link.Layer) bool {
	return false
}

func (l *Build) Backward(targets []link.Layer) {
	from := l.Info()
	for i := range targets {
		to := targets[i].Info()

		for _, link := range from.Links {
			if link.Name == to.Name {
				l.links = append(l.links, linkInfo{link, to.Share})
				l.syncs = append(l.syncs, sync.NodeLink(targets[i], sync.LinkRequire))
			}
		}

		if targets[i].Locks(l) {
			for j := range targets[i+1:] {
				if k := i + 1 + j; targets[i].Locks(targets[k]) {
					l.syncs = append(l.syncs, sync.NodeLink(targets[k], sync.LinkSerial))
				}
			}
		}

		if from.App && to.App {
			l.syncs = append(l.syncs, sync.NodeLink(targets[i], sync.LinkSerial))
		}
	}
}

func (l *Build) Forward(targets []link.Layer) {
	from := l.Info()
	for i := range targets {
		to := targets[i].Info()

		for _, link := range to.Links {
			if link.Name == from.Name {
				t := sync.LinkNone
				if link.LinkVersion {
					t = sync.LinkVersion
				}
				if link.LinkContent {
					t = sync.LinkContent
				}
				if t != sync.LinkNone {
					l.syncs = append(l.syncs, sync.NodeLink(targets[i], t))
				}
			}
		}
	}
}

func (l *Build) envs() (packfile.Envs, error) {
	out := packfile.Envs{}
	vars := struct {
		Layer string
		App   string
	}{l.LayerDir, l.AppDir}
	for _, e := range l.provide().Env.Build {
		var err error
		e.Value, err = interpolate(e.Value, vars)
		if err != nil {
			return out, err
		}
		out.Build = append(out.Build, e)
	}
	for _, e := range l.provide().Env.Launch {
		var err error
		e.Value, err = interpolate(e.Value, vars)
		if err != nil {
			return out, err
		}
		out.Launch = append(out.Launch, e)
	}
	for _, e := range l.provide().Env.Both {
		var err error
		e.Value, err = interpolate(e.Value, vars)
		if err != nil {
			return out, err
		}
		out.Both = append(out.Both, e)
	}
	return out, nil
}

func (l *Build) deps() ([]packfile.Dep, error) {
	vars, err := l.Metadata.ReadAll()
	if err != nil {
		return nil, err
	}
	var deps []packfile.Dep
	for _, dep := range l.provide().Deps {
		if dep.Name, err = interpolate(dep.Name, vars); err != nil {
			return nil, err
		}
		if dep.Version, err = interpolate(dep.Version, vars); err != nil {
			return nil, err
		}
		if dep.URI, err = interpolate(dep.URI, vars); err != nil {
			return nil, err
		}
		if dep.SHA, err = interpolate(dep.SHA, vars); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, nil
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
	return l.Layer.FindProvide()
}

func (l *Build) layerTOML() string {
	return l.LayerDir + ".toml"
}

func addRequire(md metadata.Metadata, req link.Require, dir string) error {
	reqMD := map[string]interface{}{}
	for k, v := range req.Metadata {
		if k != "launch" && k != "build" {
			reqMD[k] = v
		}
	}
	return md.WriteAll(map[string]interface{}{
		".requires": map[string]interface{}{dir: reqMD},
	})
}

func mergeRequire(md metadata.Metadata, req link.Require) error {
	prevLaunch, err := md.Read("launch")
	if err != nil {
		prevLaunch = "false"
	}
	prevBuild, err := md.Read("build")
	if err != nil {
		prevBuild = "false"
	}
	if err := md.DeleteAll(); err != nil {
		return err
	}
	if err := md.WriteAll(req.Metadata); err != nil {
		return err
	}
	nextLaunch, err := md.Read("launch")
	if err != nil {
		nextLaunch = "false"
	}
	nextBuild, err := md.Read("build")
	if err != nil {
		nextBuild = "false"
	}
	others := map[string]interface{}{}
	if mergeBoolStrings(nextLaunch, prevLaunch) {
		others["launch"] = "true"
	}
	if mergeBoolStrings(nextBuild, prevBuild) {
		others["build"] = "true"
	}
	return md.WriteAll(others)
}

func mergeBoolStrings(s1, s2 string) bool {
	return s1 == "true" || s2 == "true"
}

func padNum(n int) func(int) string {
	pad := 1 + int(math.Log10(float64(n)))
	return func(i int) string {
		return fmt.Sprintf("%0*d", pad, i)
	}
}

func (l *Build) Test() (exists, matched bool, err error) {
	if l.Layer.Require == nil {
		if err := writeLayerMetadata(l.Metadata, l.Layer); err != nil {
			return false, false, err
		}
	}
	pad := padNum(len(l.Requires))
	for i, req := range l.Requires {
		if err := addRequire(l.Metadata, req, pad(i)); err != nil {
			return false, false, err

		}
		if err := mergeRequire(l.Metadata, req); err != nil {
			return false, false, err
		}
	}

	env := packfile.NewEnvMap(os.Environ())
	md := newMetadataMap(l.Metadata)

	for _, link := range l.links {
		if l.fullEnv() && link.PathEnv != "" {
			env[link.PathEnv] = link.LayerDir
		}
		if link.VersionEnv != "" {
			lt, err := readLayerTOML(link.layerTOML())
			if err != nil {
				return false, false, err
			}
			env[link.VersionEnv] = lt.Metadata.Version
		}
		if link.MetadataEnv != "" {
			md.links[link.MetadataEnv] = link.Metadata
		}
	}
	if l.fullEnv() {
		if err := setupLinkEnv(env, l.links); err != nil {
			return false, false, err
		}
	}
	env["APP"] = l.AppDir
	if l.TestRunner != nil {
		if err := l.TestRunner.Test(l.Streamer, env, md); err != nil {
			return false, false, err
		}
	}
	if err := l.Metadata.Delete(".requires"); err != nil {
		return false, false, err
	}

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		return false, false, err
	}
	layerTOML.Cache = l.Layer.Store
	layerTOML.Build = mdToBool(l.Metadata.Read("build"))
	layerTOML.Launch = mdToBool(l.Metadata.Read("launch"))

	cachedBuildID := layerTOML.Metadata.BuildID
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
		return false, false, err
	}

	if cachedBuildID != l.LastBuildID ||
		newDigest != oldDigest ||
		newVersion != oldVersion ||
		l.provide().LockApp {
		return false, false, nil
	}
	if _, err := os.Stat(l.LayerDir); xerrors.Is(err, os.ErrNotExist) {
		return false, !l.Layer.Expose && !l.Layer.Store, nil
	}
	return true, true, nil
}

func mdToBool(s string, err error) bool {
	return err == nil && s == "true"
}

func (l *Build) Run() error {
	fmt.Fprintf(l.Stdout(), "Building layer '%s'...\n", l.Layer.Name)
	if err := os.RemoveAll(l.LayerDir); err != nil {
		return err
	}

	env := packfile.NewEnvMap(os.Environ())
	md := newMetadataMap(l.Metadata)

	for _, link := range l.links {
		if link.PathEnv != "" {
			env[link.PathEnv] = link.LayerDir
		}
		if link.VersionEnv != "" {
			lt, err := readLayerTOML(link.layerTOML())
			if err != nil {
				return err
			}
			env[link.VersionEnv] = lt.Metadata.Version
		}
		if link.MetadataEnv != "" {
			md.links[link.MetadataEnv] = link.Metadata
		}
	}
	if err := setupLinkEnv(env, l.links); err != nil {
		return err
	}
	if err := os.MkdirAll(l.LayerDir, 0777); err != nil {
		return err
	}
	if err := l.setupEnvs(env); err != nil {
		return err
	}
	if err := l.setupProfile(); err != nil {
		return err
	}
	deps, err := l.deps()
	if err != nil {
		return err
	}

	env["APP"] = l.AppDir
	env["LAYER"] = l.LayerDir
	if l.ProvideRunner != nil {
		if err := l.ProvideRunner.Provide(l.Streamer, env, md, deps); err != nil {
			return err
		}
	}

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		return err
	}
	versionStr := "."
	if v, err := md.Read("version"); err == nil {
		versionStr = " with version: " + v
	}
	fmt.Fprintf(l.Stdout(), "Built layer '%s'%s\n", l.Layer.Name, versionStr)
	saved, err := l.Metadata.ReadAll()
	if err != nil {
		return err
	}
	delete(saved, "launch")
	delete(saved, "build")
	layerTOML.Metadata.Saved = saved
	return writeTOML(layerTOML, layerTOMLPath)
}

func (l *Build) Skip() error {
	fmt.Fprintf(l.Stdout(), "Skipping layer '%s'.\n", l.Layer.Name)

	layerTOMLPath := l.LayerDir + ".toml"
	layerTOML, err := readLayerTOML(layerTOMLPath)
	if err != nil {
		return err
	}
	if err := l.Metadata.DeleteAll(); err != nil {
		return err
	}
	saved := layerTOML.Metadata.Saved
	if layerTOML.Launch {
		saved["launch"] = "true"
	}
	if layerTOML.Build {
		saved["build"] = "true"
	}
	return l.Metadata.WriteAll(saved)
}

func (l *Build) digest() string {
	hash := sha256.New()
	writeField(hash, "build")
	writeField(hash, l.Layer.Version, l.Layer.Metadata)

	writeField(hash, l.ProvideRunner.Version())

	if deps, err := l.deps(); err == nil {
		for _, dep := range deps {
			writeField(hash, dep.Name, dep.Version, dep.URI, dep.SHA, dep.Metadata)
		}
	}
	for _, file := range l.provide().Profile {
		writeField(hash, file.Inline)
		writeFile(hash, file.Path)
	}
	if envs, err := l.envs(); err == nil {
		for _, env := range envs.Launch {
			writeField(hash, env.Name, env.Value, env.Op, env.Delim)
		}
		for _, env := range envs.Build {
			writeField(hash, env.Name, env.Value, env.Op, env.Delim)
		}
		for _, env := range envs.Both {
			writeField(hash, env.Name, env.Value, env.Op, env.Delim)
		}
	}
	for _, link := range l.provide().Links {
		writeField(hash, link.Name, link.PathEnv, link.VersionEnv, link.MetadataEnv)
		fmt.Fprintf(hash, "%t\n%t\n", link.LinkContent, link.LinkVersion)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeField(out io.Writer, values ...interface{}) {
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

func (l *Build) setupEnvs(env packfile.EnvMap) error {
	envs, err := l.envs()
	if err != nil {
		return err
	}
	envBoth := filepath.Join(l.LayerDir, "env")
	envBuild := filepath.Join(l.LayerDir, "env.build")
	envLaunch := filepath.Join(l.LayerDir, "env.launch")
	if err := setupEnvDir(envs.Both, envBoth); err != nil {
		return err
	}
	if err := setupEnvDir(envs.Build, envBuild); err != nil {
		return err
	}
	if err := setupEnvDir(envs.Launch, envLaunch); err != nil {
		return err
	}
	lcEnv := lcenv.Env{RootDirMap: lcenv.POSIXBuildEnv, Vars: env}
	if err := lcEnv.AddEnvDir(envBoth); err != nil {
		return err
	}
	return lcEnv.AddEnvDir(envBuild)
}

func setupEnvDir(env []packfile.Env, path string) error {
	if len(env) == 0 {
		return nil
	}
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

func (l *Build) setupProfile() error {
	profiles := l.provide().Profile
	if len(profiles) == 0 {
		return nil
	}
	pad := padNum(len(profiles))
	profiled := filepath.Join(l.LayerDir, "profile.d")
	if err := os.Mkdir(profiled, 0777); err != nil {
		return err
	}
	for i, file := range profiles {
		path := filepath.Join(profiled, pad(i)+".sh")
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

func setupLinkEnv(env packfile.EnvMap, links []linkInfo) error {
	lcEnv := lcenv.Env{RootDirMap: lcenv.POSIXBuildEnv, Vars: env}
	for _, link := range links {
		if err := lcEnv.AddRootDir(link.LayerDir); err != nil {
			return err
		}
	}
	for _, link := range links {
		if err := lcEnv.AddEnvDir(filepath.Join(link.LayerDir, "env")); err != nil {
			return err
		}
		if err := lcEnv.AddEnvDir(filepath.Join(link.LayerDir, "env.build")); err != nil {
			return err
		}
	}
	return nil
}

type metadataMap struct {
	metadata.Metadata
	links map[string]metadata.Metadata
}

func (m metadataMap) Link(as string) metadata.Metadata {
	return m.links[as]
}

func (m metadataMap) Dir() string {
	return m.Metadata.(interface{ Dir() string }).Dir()
}

func newMetadataMap(md metadata.Metadata) metadataMap {
	return metadataMap{md, map[string]metadata.Metadata{}}
}
