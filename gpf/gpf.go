package gpf

import "github.com/sclevine/packfile/layer"

type BP struct{}

type Packfile struct {
	Config    Config    `toml:"config"`
	Processes []Process `toml:"processes"`
	Caches    []Cache   `toml:"caches"`
	Layers    []Layer   `toml:"layers"`
}

type Config struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	Name    string `toml:"name"`
	Serial  bool   `toml:"serial"`
}

type Process struct {
	Type    string   `toml:"type"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Direct  bool     `toml:"direct"`
}

//func (bp *BP) Cache(name string, setup func() error) {
//
//}

type Cache interface {
	Name() string
	Setup() error
}

type Layer interface {
	Name() string
	Config() *LayerConfig
	Require() error
	Provide() error
	Build() error
}

type LayerConfig struct {
	Export   bool
	Expose   bool
	Store    bool
	Version  string
	Metadata map[string]string
}


type Provide struct {
	Exec
	Test     Exec         `toml:"test"`
	Links    []layer.Link `toml:"links"`
	Deps     []Dep        `toml:"deps"`
	Env      Envs         `toml:"env"`
	Profile  []File       `toml:"profile"`
	WriteApp bool         `toml:"write-app"`
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

type Dep struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
	URI     string `toml:"uri"`
}
