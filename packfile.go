package packfile

const DefaultShell = "/usr/bin/env bash"

type Packfile struct {
	Config    Config    `toml:"config"`
	Processes []Process `toml:"processes"`
	Caches    []Cache   `toml:"caches"`
	Layers    []Layer   `toml:"layers"`
	Slices    []Slice   `toml:"slices"`
}

type Config struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	Name    string `toml:"name"`
	Shell   string `toml:"shell"`
	Serial  bool   `toml:"serial"` // TODO
}

type Process struct {
	Type    string   `toml:"type"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Direct  bool     `toml:"direct"`
}

type Slice struct {
	Paths []string `tom:"paths"`
}

type Cache struct {
	Name  string `toml:"name"`
	Setup *Exec  `toml:"setup"`
}

type Layer struct {
	Name     string            `toml:"name"`
	Export   bool              `toml:"export"`
	Expose   bool              `toml:"expose"`
	Store    bool              `toml:"store"`
	Version  string            `toml:"version"`
	Metadata map[string]string `toml:"metadata"`
	Require  *Require          `toml:"require"`
	Provide  *Provide          `toml:"provide"`
	Build    *Provide          `toml:"build"`
}

type Require struct {
	Exec
}

type Provide struct {
	Exec
	WriteApp bool   `toml:"write-app"`
	Test     *Test  `toml:"test"`
	Links    []Link `toml:"links"`
	Deps     []Dep  `toml:"deps"`
	Env      Envs   `toml:"env"`
	Profile  []File `toml:"profile"`
}

type Exec struct {
	Shell  string `toml:"shell"`
	Inline string `toml:"inline"`
	Path   string `toml:"path"`
}

type Test struct {
	Exec
	FullEnv bool `toml:"full-env"`
}

type Link struct {
	Name        string `toml:"name"`
	PathEnv     string `toml:"path-as"`
	VersionEnv  string `toml:"version-as"`
	MetadataEnv string `toml:"metadata-as"`
	LinkContent bool   `toml:"link-content"`
	LinkVersion bool   `toml:"link-version"`
}

type Dep struct {
	Name     string                 `toml:"name"`
	Version  string                 `toml:"version"`
	URI      string                 `toml:"uri"`
	SHA      string                 `toml:"sha"`
	Metadata map[string]interface{} `toml:"metadata"`
}

type Envs struct {
	Build  []Env `toml:"build"`
	Launch []Env `toml:"launch"`
}

type Env struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

type File struct {
	Inline string `toml:"inline"`
	Path   string `toml:"path"`
}
