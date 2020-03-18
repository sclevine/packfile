package packfile

import (
	"io"
	"strings"

	"github.com/sclevine/packfile/metadata"
)

type SetupRunner interface {
	Setup(st Streamer, env EnvMap) error
	Version() string
}

type RequireRunner interface {
	Require(st Streamer, env EnvMap, md Metadata) error
}

type TestRunner interface {
	Test(st Streamer, env EnvMap, md Metadata) error
}

type ProvideRunner interface {
	Provide(st Streamer, env EnvMap, md Metadata, deps []Dep) error
	Version() string
}

type Streamer interface {
	Stdout() io.Writer
	Stderr() io.Writer
}

type Metadata interface {
	metadata.Metadata
	Link(as string) metadata.Metadata
}

type EnvMap map[string]string

func (e EnvMap) Environ() []string {
	var out []string
	for k, v := range e {
		out = append(out, k+"="+v)
	}
	return out
}

func NewEnvMap(env []string) EnvMap {
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
