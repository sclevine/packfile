package metadata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
)

var ErrNoKeys = xerrors.New("no keys")

func NewFS(path string) Store {
	return fsStore{path}
}

type fsStore struct {
	path string
}

func (fs fsStore) Read(keys ...string) (string, error) {
	if len(keys) == 0 {
		return "", ErrNoKeys
	}
	value, err := ioutil.ReadFile(filepath.Join(keys...))
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(value), "\n"), nil
}

func (fs fsStore) ReadAll() (map[string]interface{}, error) {
	metadata := map[string]interface{}{}
	return metadata, eachFile(fs.path, metadata, func(name string, m map[string]interface{}) error {
		var err error
		m[name], err = fs.Read(name)
		return err
	})
}

func (fs fsStore) Delete(keys ...string) error {
	return os.RemoveAll(fs.keyPath(keys...))
}

func (fs fsStore) DeleteAll() error {
	files, err := ioutil.ReadDir(fs.path)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := fs.Delete(f.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (fs fsStore) Write(value string, keys ...string) error {
	if err := fs.Delete(keys...); err != nil {
		return err
	}
	return ioutil.WriteFile(fs.keyPath(keys...), []byte(value), 0666)
}

func (fs fsStore) keyPath(keys ...string) string {
	return filepath.Join(append([]string{fs.path}, keys...)...)
}

//func (fs fsStore) WriteAll(metadata map[string]interface{}) error {
//	return eachKey(metadata, fs.path, func(name, value, dir string) error {
//		return ioutil.WriteFile(filepath.Join(dir, name), []byte(value), 0666)
//	})
//}

func (fs fsStore) WriteAll(metadata map[string]interface{}) error {
	return eachKey(metadata, nil, func(value string, keys ...string) error {
		return fs.Write(value, keys...)
	})
}

func writeLayer(fs fsStore, layer *packfile.Layer) error {
	if err := fs.WriteAll(layer.Metadata); err != nil {
		return err
	}
	return fs.WriteAll(map[string]interface{}{
		"version": layer.Version,
		"launch":  fmt.Sprintf("%t", layer.Export),
		"build":   fmt.Sprintf("%t", layer.Expose),
	})
}

//func copyStringMap(m map[string]string) map[string]string {
//	out := map[string]string{}
//	for k, v := range m {
//		out[k] = v
//	}
//	return out
//}

func copyMap(m map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			out[k] = copyMap(vm)
		} else {
			out[k] = v
		}
	}
	return out
}

func eachFile(dir string, m map[string]interface{}, fn func(name string, m map[string]interface{}) error) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			n := map[string]interface{}{}
			m[f.Name()] = n
			return eachFile(filepath.Join(dir, f.Name()), n, fn)
		}
		if err := fn(f.Name(), m); err != nil {
			return err
		}
	}
	return nil
}

//func eachKey(m map[string]interface{}, dir string, fn func(k, v, dir string) error) error {
//	for k, v := range m {
//		switch v := v.(type) {
//		case string:
//			return fn(k, v, dir)
//		case map[string]interface{}:
//			return eachKey(v, filepath.Join(dir, k), fn)
//		}
//	}
//	return nil
//}

func eachKey(m map[string]interface{}, start []string, fn func(v string, keys ...string) error) error {
	for k, v := range m {
		switch v := v.(type) {
		case string:
			return fn(v, append(start, k)...)
		case map[string]interface{}:
			return eachKey(v, append(start, k), fn)
		}
	}
	return nil
}
