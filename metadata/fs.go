package metadata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
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
	value, err := ioutil.ReadFile(fs.keyPath(keys...))
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(value), "\n"), nil
}

func (fs fsStore) ReadAll() (map[string]interface{}, error) {
	metadata := map[string]interface{}{}
	return metadata, fs.eachFile(metadata, nil, func(m map[string]interface{}, keys ...string) error {
		var err error
		m[keys[len(keys)-1]], err = fs.Read(keys...)
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
		return ErrNotKey
	}
	if len(keys) > 1 {
		if err := os.MkdirAll(filepath.Dir(fs.keyPath(keys...)), 0777); err != nil {
			return err
		}
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

func (fs fsStore) Dir() string {
	return fs.path
}

func (fs fsStore) eachFile(m map[string]interface{}, start []string, fn func(m map[string]interface{}, keys ...string) error) error {
	files, err := ioutil.ReadDir(fs.keyPath(start...))
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			n := map[string]interface{}{}
			m[f.Name()] = n
			return fs.eachFile(n, append(start, f.Name()), fn)
		}
		if err := fn(m, append(start, f.Name())...); err != nil {
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
	case nil:
		return "", ErrNotExist
	default:
		return "", ErrNotValue
	}
}