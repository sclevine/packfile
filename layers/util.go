package layers

import (
	"errors"
	"io"
	"os"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/exec"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type Streamer interface {
	Stdout() io.Writer
	Stderr() io.Writer
	Stream(out, err io.Writer) error
	Close() error
}

type LinkLayer interface {
	linker
	sync.Runner
	Stream(out, err io.Writer) error
	Close() error
}

type linker interface {
	info() linkerInfo
	locks(target linker) bool
	forward(targets []linker, syncs []*sync.Layer)
	backward(targets []linker, syncs []*sync.Layer)
}

type LinkShare struct {
	LayerDir string
	Metadata metadata.Metadata
	Err      error
}

type linkInfo struct {
	packfile.Link
	*LinkShare
}

func (l linkInfo) layerTOML() string {
	return l.LayerDir + ".toml"
}

type linkerInfo struct {
	name  string
	share *LinkShare
	links []packfile.Link
	app   bool
}

func writeLayerMetadata(md metadata.Metadata, layer *packfile.Layer) error {
	if err := md.WriteAll(layer.Metadata); err != nil {
		return err
	}
	others := map[string]interface{}{}
	if layer.Version != "" {
		others["version"] = layer.Version
	}
	if layer.Export {
		others["launch"] = "true"
	}
	if layer.Expose {
		others["build"] = "true"
	}
	return md.WriteAll(others)
}

func LinkLayers(layers []LinkLayer) []*sync.Layer {
	lock := sync.NewLock(len(layers))
	syncs := make([]*sync.Layer, len(layers))
	for i := range layers {
		syncs[i] = sync.NewLayer(lock, layers[i])
	}
	for i := range layers {
		layers[i].backward(toLinkers(layers[:i]), syncs[:i])
	}
	for i := range layers {
		layers[i].forward(toLinkers(layers[i+1:]), syncs[i+1:])
	}
	return syncs
}

func toLinkers(layers []LinkLayer) []linker {
	out := make([]linker, len(layers))
	for i, layer := range layers {
		out[i] = layer
	}
	return out
}

type Require struct {
	Name     string                 `toml:"name"`
	Version  string                 `toml:"version"`
	Metadata map[string]interface{} `toml:"metadata"`
}

func ReadRequires(layers []LinkLayer) ([]Require, error) {
	var requires []Require
	for _, layer := range layers {
		info := layer.info()
		if exec.IsFail(info.share.Err) {
			continue
		} else if info.share.Err != nil {
			return nil, xerrors.Errorf("error for layer '%s': %w", info.name, info.share.Err)
		}
		if info.share.Metadata == nil {
			continue
		}
		req, err := readRequire(info.name, info.share.Metadata)
		if err != nil {
			return nil, xerrors.Errorf("invalid metadata for layer '%s': %w", info.name, err)
		}
		requires = append(requires, req)
	}
	return requires, nil
}

func readRequire(name string, md metadata.Metadata) (Require, error) {
	out := Require{Name: name}
	var err error
	if out.Metadata, err = md.ReadAll(); err != nil {
		return Require{}, err
	}
	if v, ok := out.Metadata["version"]; ok {
		if out.Version, ok = v.(string); !ok {
			return Require{}, errors.New("version must be a string")
		}
	}
	delete(out.Metadata, "version")
	return out, nil
}

func writeTOML(v interface{}, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(v)
}