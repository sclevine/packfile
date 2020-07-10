package packfile

const DefaultShell = "/usr/bin/env bash"

type Packfile struct {
	API       string    `toml:"api" yaml:"api"`
	Config    Config    `toml:"config" yaml:"config"`
	Processes []Process `toml:"processes" yaml:"processes"`
	Caches    []Cache   `toml:"caches" yaml:"caches"`
	Layers    []Layer   `toml:"layers" yaml:"layers"`
	Slices    []Slice   `toml:"slices" yaml:"slices"`
	Stacks    []Stack   `toml:"stacks" yaml:"stacks"`
}

type Config struct {
	ID      string `toml:"id" yaml:"id"`
	Version string `toml:"version" yaml:"version"`
	Name    string `toml:"name" yaml:"name"`
	Shell   string `toml:"shell" yaml:"shell"`
}

type Process struct {
	Type    string   `toml:"type" yaml:"type"`
	Command string   `toml:"command" yaml:"command"`
	Args    []string `toml:"args" yaml:"args"`
	Direct  bool     `toml:"direct" yaml:"direct"`
}

type Slice struct {
	Paths []string `toml:"paths" yaml:"paths"`
}

type Stack struct {
	ID     string   `toml:"id" yaml:"id"`
	Mixins []string `toml:"mixins,omitempty" yaml:"mixins,omitempty"`
}

type Cache struct {
	Name  string `toml:"name" yaml:"name"`
	Setup *Setup `toml:"setup" yaml:"setup"`
}

type Setup struct {
	Exec   `yaml:",inline"`
	Runner SetupRunner `toml:"-" yaml:"-"`
}

type Layer struct {
	Name     string                 `toml:"name" yaml:"name"`
	Export   bool                   `toml:"export" yaml:"export"`
	Expose   bool                   `toml:"expose" yaml:"expose"`
	Store    bool                   `toml:"store" yaml:"store"`
	Version  string                 `toml:"version" yaml:"version"`
	Metadata map[string]interface{} `toml:"metadata" yaml:"metadata"`
	Require  *Require               `toml:"require" yaml:"require"`
	Provide  *Provide               `toml:"provide" yaml:"provide"`
	Build    *Provide               `toml:"build" yaml:"build"`
}

func (l *Layer) FindProvide() *Provide {
	if l.Provide != nil {
		return l.Provide
	}
	return l.Build
}

type Require struct {
	Exec   `yaml:",inline"`
	Runner RequireRunner `toml:"-" yaml:"-"`
}

type Provide struct {
	LockApp bool   `toml:"lock-app" yaml:"lockApp"`
	Test    *Test  `toml:"test" yaml:"test"`
	Run     *Run   `toml:"run" yaml:"run"`
	Links   []Link `toml:"links" yaml:"links"`
	Deps    []Dep  `toml:"deps" yaml:"deps"`
	Env     Envs   `toml:"env" yaml:"env"`
	Profile []File `toml:"profile" yaml:"profile"`
}

type Exec struct {
	Shell  string `toml:"shell" yaml:"shell"`
	Inline string `toml:"inline" yaml:"inline"`
	Path   string `toml:"path" yaml:"path"`
}

type Run struct {
	Exec   `yaml:",inline"`
	Runner ProvideRunner `toml:"-" yaml:"-"`
}

type Test struct {
	Exec    `yaml:",inline"`
	Runner  TestRunner `toml:"-" yaml:"-"`
	Match   []string   `toml:"match" yaml:"match"`
	FullEnv bool       `toml:"full-env" yaml:"fullEnv"`
}

type Link struct {
	Name        string `toml:"name" yaml:"name"`
	PathEnv     string `toml:"path-as" yaml:"pathAs"`
	VersionEnv  string `toml:"version-as" yaml:"versionAs"`
	MetadataEnv string `toml:"metadata-as" yaml:"metadataAs"`
	LinkContent bool   `toml:"link-content" yaml:"linkContent"`
	LinkVersion bool   `toml:"link-version" yaml:"linkVersion"`
}

type Dep struct {
	Name     string                 `toml:"name" yaml:"name"`
	Version  string                 `toml:"version" yaml:"version"`
	URI      string                 `toml:"uri" yaml:"uri"`
	SHA      string                 `toml:"sha" yaml:"sha"`
	Metadata map[string]interface{} `toml:"metadata" yaml:"metadata"`
}

type Envs struct {
	Build  []Env `toml:"build" yaml:"build"`
	Launch []Env `toml:"launch" yaml:"launch"`
	Both   []Env `toml:"both" yaml:"both"`
}

type Env struct {
	Name  string `toml:"name" yaml:"name"`
	Value string `toml:"value" yaml:"value"`
	Op    string `toml:"op" yaml:"op"`
	Delim string `toml:"delim" yaml:"delim"`
}

type File struct {
	Inline string `toml:"inline" yaml:"inline"`
	Path   string `toml:"path" yaml:"path"`
}

type ConfigTOML struct {
	ContextDir  string `toml:"context-dir" yaml:"contextDir"`
	StoreDir    string `toml:"store-dir" yaml:"storeDir"`
	MetadataDir string `toml:"metadata-dir" yaml:"metadataDir"`
	Deps        []Dep  `toml:"deps" yaml:"deps"`
}
