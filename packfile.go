package packfile

type Packfile struct {
	Config    Config    `toml:"config"`
	Processes []Process `toml:"processes"`
	Layers    []Layer   `toml:"layers"`
}

type Config struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
	Name    string `toml:"name"`
	Shell   string `toml:"shell"`
	Serial  bool   `toml:"serial"`
}

type Process struct {
	Type    string   `toml:"type"`
	Command string   `toml:"command"`
	Args    []string `toml:"args"`
	Direct  bool     `toml:"direct"`
}

type Layer struct {
	Name     string            `toml:"name"`
	Use      bool              `toml:"use"`
	Cache    bool              `toml:"cache"`
	Expose   bool              `toml:"expose"`
	Version  string            `toml:"version"`
	Metadata map[string]string `toml:"metadata"`
	Detect   *DetectAction     `toml:"detect"`
	Build    *BuildAction      `toml:"build"`
	Launch   *LaunchAction     `toml:"launch"`
}

type DetectAction struct {
	Exec
	Require []DetectRequire `toml:"require"`
}

type BuildAction struct {
	Exec
	Require  []BuildRequire `toml:"require"`
	Deps     []Dep          `toml:"deps"`
	Env      []Env          `toml:"env"`
	WriteApp bool           `toml:"write-app"`
}

type DetectRequire struct {
	Name        string `toml:"name"`
	VersionEnv  string `toml:"version-as"`
	MetadataEnv string `toml:"metadata-as"`
}

type BuildRequire struct {
	Name        string `toml:"name"`
	Write       bool   `toml:"write"`
	PathEnv     string `toml:"path-as"`
	VersionEnv  string `toml:"version-as"`
	MetadataEnv string `toml:"metadata-as"`
}

type LaunchAction struct {
	Profile []File `toml:"profile"`
	Env     []Env  `toml:"env"`
}

type Exec struct {
	Shell  string `toml:"shell"`
	Inline string `toml:"inline"`
	Path   string `toml:"path"`
	Func   string `toml:"func"`
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
