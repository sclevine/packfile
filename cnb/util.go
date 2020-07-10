package cnb

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/layers"
	"github.com/sclevine/packfile/link"
)

func writeTOML(lt interface{}, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(lt)
}

func eachDir(dir string, fn func(name string) error) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if err := fn(f.Name()); err != nil {
			return err
		}
	}
	return nil
}

func shellOverride(exec packfile.Exec, shell string) packfile.Exec {
	if exec.Shell == "" {
		exec.Shell = shell
	}
	return exec
}

func toLinkLayers(layers []layers.StreamLayer) []link.Layer {
	out := make([]link.Layer, len(layers))
	for i, layer := range layers {
		out[i] = layer
	}
	return out
}

type matchTest struct {
	Globs []string
	Dir string
}

func (m matchTest) Test(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata) error {
	hash := sha256.New()
	for _, glob := range m.Globs {
		matches, err := filepath.Glob(filepath.Join(m.Dir, glob))
		if err != nil {
			return err
		}
		for _, match := range matches {
			if err := sumPath(hash, match); err != nil {
				return err
			}
		}
	}
	return md.Write(fmt.Sprintf("%x", hash.Sum(nil)), "version")
}

func sumPath(hash hash.Hash, path string) error {
	return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if _, err := io.WriteString(hash, path); err != nil {
			return err
		}
		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(hash, f); err != nil {
				return err
			}
		}
		return nil
	})
}