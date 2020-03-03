package layers

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/metadata"
	"github.com/sclevine/packfile/sync"
)

type Streamer interface {
	Writers() (out, err io.Writer)
	Stream(out, err io.Writer)
	Flush()
	Close()
}

type LinkLayer interface {
	linker
	sync.Runner
	Stream(out, err io.Writer)
	Close()
}

type linker interface {
	info() linkerInfo
	locks(target linker) bool
	forward(targets []linker, syncs []*sync.Layer)
	backward(targets []linker, syncs []*sync.Layer)
}

type LinkShare struct {
	LayerDir string
	Metadata metadata.Store
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

type CodeError int

func (e CodeError) Error() string {
	return fmt.Sprintf("failed with code %d", e)
}

func IsFail(err error) bool {
	var e CodeError
	if xerrors.As(err, &e) {
		return e == 100
	}
	return false
}

func IsError(err error) bool {
	var e CodeError
	if xerrors.As(err, &e) {
		return e != 100
	}
	return false
}

func writeLayerMetadata(store metadata.Store, layer *packfile.Layer) error {
	if err := store.WriteAll(layer.Metadata); err != nil {
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
	return store.WriteAll(others)
}

// NOTE: implements UNIX exec-style shebang parsing for shell
func execCmd(e *packfile.Exec, ctxDir, shell string) (*exec.Cmd, io.Closer, error) {
	if e.Inline != "" && e.Path != "" {
		return nil, nil, xerrors.New("both inline and path specified")
	}
	if e.Shell != "" {
		shell = e.Shell
	}
	parts := strings.SplitN(shell, " ", 2)
	if len(parts) == 0 {
		return nil, nil, xerrors.New("missing shell")
	}
	var args []string
	if len(parts) > 1 {
		shell = parts[0]
		args = append(args, parts[1])
	}
	if e.Inline != "" {
		f, err := ioutil.TempFile("", "packfile.")
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		if _, err := f.WriteString(e.Inline); err != nil {
			return nil, nil, err
		}
		return exec.Command(shell, append(args, f.Name())...), rmCloser{f.Name()}, nil
	}

	if e.Path == "" {
		return nil, nil, xerrors.New("missing executable")
	}

	return exec.Command(shell, append(args, filepath.Join(ctxDir, e.Path))...), nopCloser{}, nil
}

type rmCloser struct{ path string }

func (c rmCloser) Close() error { return os.Remove(c.path) }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

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
		if IsFail(info.share.Err) {
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

func readRequire(name string, metadata metadata.Store) (Require, error) {
	out := Require{Name: name}
	var err error
	if out.Metadata, err = metadata.ReadAll(); err != nil {
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
