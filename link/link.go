package link

import (
	"io"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/exec"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type Layer interface {
	sync.Node
	Streamer
	Info() Info
	Locks(target Layer) bool
	Forward(targets []Layer)
	Backward(targets []Layer)
}

type Streamer interface {
	Stdout() io.Writer
	Stderr() io.Writer
	Stream(out, err io.Writer) error
	Close() error
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
}

func Layers(layers []Layer) {
	for i := range layers {
		layers[i].Backward(layers[:i])
	}
	for i := range layers {
		layers[i].Forward(layers[i+1:])
	}
}

type Require struct {
	Name     string                 `toml:"name"`
	Metadata map[string]interface{} `toml:"metadata"`
}

func Requires(layers []Layer) ([]Require, error) {
	var requires []Require
	for _, layer := range layers {
		info := layer.Info()
		err := sync.NodeError(layer)
		if exec.IsFail(err) {
			continue
		} else if err != nil {
			return nil, xerrors.Errorf("error for layer '%s': %w", info.Name, err)
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
	return out, nil
}
