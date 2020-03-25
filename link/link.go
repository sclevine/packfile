package link

import (
	"errors"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/exec"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type Layer interface {
	sync.Runner
	Info() Info
	Locks(target Layer) bool
	Forward(targets []Layer, syncs []*sync.Layer)
	Backward(targets []Layer, syncs []*sync.Layer)
}

type Info struct {
	Name  string
	Share *Share
	Links []packfile.Link
	App   bool
}

type Share struct {
	LayerDir string
	Metadata metadata.Metadata
	Err      error
}

func Layers(layers []Layer) []*sync.Layer {
	lock := sync.NewLock(len(layers))
	syncs := make([]*sync.Layer, len(layers))
	for i := range layers {
		syncs[i] = sync.NewLayer(lock, layers[i])
	}
	for i := range layers {
		layers[i].Backward(layers[:i], syncs[:i])
	}
	for i := range layers {
		layers[i].Forward(layers[i+1:], syncs[i+1:])
	}
	return syncs
}

type Require struct {
	Name     string                 `toml:"name"`
	Version  string                 `toml:"version"`
	Metadata map[string]interface{} `toml:"metadata"`
}

func Requires(layers []Layer) ([]Require, error) {
	var requires []Require
	for _, layer := range layers {
		info := layer.Info()
		if exec.IsFail(info.Share.Err) {
			continue
		} else if info.Share.Err != nil {
			return nil, xerrors.Errorf("error for layer '%s': %w", info.Name, info.Share.Err)
		}
		if info.Share.Metadata == nil {
			continue
		}
		req, err := readRequire(info.Name, info.Share.Metadata)
		if err != nil {
			return nil, xerrors.Errorf("invalid metadata for layer '%s': %w", info.Name, err)
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
