package layers

import (
	"io"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/metadata"
)

type Streamer interface {
	Stdout() io.Writer
	Stderr() io.Writer
	Stream(out, err io.Writer) error
	Close() error
}

type StreamLayer interface {
	link.Layer
	Streamer
}

type linkInfo struct {
	packfile.Link
	*link.Share
}

func (l linkInfo) layerTOML() string {
	return l.LayerDir + ".toml"
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

func writeTOML(v interface{}, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(v)
}