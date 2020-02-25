package metadata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/sclevine/packfile"
)

func NewFS(path string) Store {
	return fsStore{path}
}

type fsStore struct {
	path string
}

func (fs fsStore) Read(key string) string {
	value, err := ioutil.ReadFile(filepath.Join(fs.path, key))
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(value), "\n")
}

func (fs fsStore) ReadAll() (map[string]string, error) {
	metadata := map[string]string{}
	return metadata, eachFile(fs.path, func(name string) error {
		metadata[name] = fs.Read(name)
		return nil
	})
}

func (fs fsStore) DeleteAll() error {
	return eachFile(fs.path, func(name string) error {
		return os.Remove(filepath.Join(fs.path, name))
	})
}

func (fs fsStore) WriteAll(metadata map[string]string) error {
	for k, v := range metadata {
		if err := ioutil.WriteFile(filepath.Join(fs.path, k), []byte(v), 0666); err != nil {
			return err
		}
	}
	return nil
}

func (fs fsStore) WriteLayer(layer *packfile.Layer) error {
	md := copyMap(layer.Metadata)
	md["version"] = layer.Version
	if err := fs.WriteAll(md); err != nil {
		return err
	}
	return fs.WriteAll(map[string]string{
		"launch": fmt.Sprintf("%t", layer.Export),
		"build":  fmt.Sprintf("%t", layer.Expose),
	})
}

func copyMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func eachFile(dir string, fn func(name string) error) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if err := fn(f.Name()); err != nil {
			return err
		}
	}
	return nil
}