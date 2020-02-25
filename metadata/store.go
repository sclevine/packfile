package metadata

import "github.com/sclevine/packfile"

type Store interface {
	Read(keys ...string) (string, error)
	ReadAll() (map[string]interface{}, error)
	DeleteAll() error
	WriteAll(metadata map[string]interface{}) error
	WriteLayer(layer *packfile.Layer) error
}
