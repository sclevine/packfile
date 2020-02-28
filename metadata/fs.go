package metadata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/sclevine/packfile"
)


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
	if len(keys) == 0 {
		return ErrNoKeys
	}
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
	if len(keys) == 0 {
		return ErrNoKeys
	}
	if err := fs.Delete(keys...); err != nil {
		return err
	}
	return ioutil.WriteFile(fs.keyPath(keys...), []byte(value), 0666)
}

func (fs fsStore) keyPath(keys ...string) string {
	return filepath.Join(append([]string{fs.path}, keys...)...)
}

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

func eachKey(m map[string]interface{}, start []string, fn func(v string, keys ...string) error) error {
	for k, v := range m {
		switch v := v.(type) {
		case map[string]interface{}:
			return eachKey(v, append(start, k), fn)
		default:
			s, err := primToString(v)
			if err != nil {
				return err
			}
			return fn(s, append(start, k)...)
		}
	}
	return nil
}

func primToString(v interface{}) (string, error) {
	switch v := v.(type) {
	case toml.TextMarshaler:
		text, err := v.MarshalText()
		if err != nil {
			return "", err
		}
		return string(text), nil
	case fmt.Stringer:
		return v.String(), nil
	case string:
		return v, nil
	case bool:
		return fmt.Sprintf("%v", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case float64:
		return fmt.Sprintf("%f", v), nil
	default:
		return "", ErrNotValue
	}
}