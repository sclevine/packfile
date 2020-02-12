package layers

import (
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
	"github.com/sclevine/packfile/sync"
)

type Streamer interface {
	Writers() (out, err io.Writer)
	Stream(out, err io.Writer)
	Close()
}

type LinkLayer interface {
	sync.Runner
	Stream(out, err io.Writer)
	Close()
	info() layerInfo
	locks(target LinkLayer) bool
	forward(targets []LinkLayer, syncs []*sync.Layer)
	backward(targets []LinkLayer, syncs []*sync.Layer)
}

type LinkShare struct {
	LayerDir    string
	MetadataDir string
	Err         error
}

type linkInfo struct {
	packfile.Link
	*LinkShare
}

func (l linkInfo) layerTOML() string {
	return l.LayerDir + ".toml"
}

type layerInfo struct {
	name  string
	share *LinkShare
	links []packfile.Link
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

func writeMetadata(path, version string, metadata map[string]string) error {
	for k, v := range metadata {
		if err := ioutil.WriteFile(filepath.Join(path, k), []byte(v), 0666); err != nil {
			return err
		}
	}
	if version == "" {
		return nil
	}
	return ioutil.WriteFile(filepath.Join(path, "version"), []byte(version), 0666)
}

// NOTE: implements UNIX exec-style shebang parsing for shell
func execCmd(e *packfile.Exec, shell string) (*exec.Cmd, io.Closer, error) {
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

	return exec.Command(shell, append(args, e.Path)...), nopCloser{}, nil
}

type rmCloser struct{ path string }

func (c rmCloser) Close() error { return os.Remove(c.path) }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func LinkLayers(layers []LinkLayer) []*sync.Layer {
	lock := sync.NewLock(len(layers))
	out := make([]*sync.Layer, len(layers))
	for i := range layers {
		out[i] = sync.NewLayer(lock, layers[i])
	}
	for i := range layers {
		layers[i].backward(layers[:i], out[:i])
	}
	//for i := range layers {
	//	layers[i].forward(layers[i+1:], out[i+1:])
	//}
	return out
}

type Require struct {
	Name     string            `toml:"name"`
	Version  string            `toml:"version"`
	Metadata map[string]string `toml:"metadata"` // TODO: fails to accept all metadata at build
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
		if info.share.MetadataDir == "" {
			continue
		}
		req, err := readRequire(info.name, info.share.MetadataDir)
		if err != nil {
			return nil, xerrors.Errorf("invalid metadata for layer '%s': %w", info.name, err)
		}
		requires = append(requires, req)
	}
	return requires, nil
}

func eachFile(dir string, fn func(name, path string) error) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if err := fn(f.Name(), filepath.Join(dir, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

func readRequire(name, path string) (Require, error) {
	out := Require{
		Name:     name,
		Metadata: map[string]string{},
	}
	if err := eachFile(path, func(name, path string) error {
		value, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		if name == "version" {
			out.Version = string(value)
		} else {
			out.Metadata[name] = string(value)
		}
		return nil
	}); err != nil {
		return Require{}, err
	}
	return out, nil
}

func writeTOML(lt interface{}, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(lt)
}
