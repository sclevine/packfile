package metadata

import "github.com/sclevine/packfile"

type Store interface {
	Read(key string) string
	ReadAll() (map[string]string, error)
	DeleteAll() error
	WriteAll(metadata map[string]string) error
	WriteLayer(layer *packfile.Layer) error
}
