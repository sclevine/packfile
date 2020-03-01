package metadata

import "golang.org/x/xerrors"

type Store interface {
	Read(keys ...string) (string, error)
	ReadAll() (map[string]interface{}, error)

	// Delete only returns an error if len(keys) == 0 or if the item exists and can't be deleted
	Delete(keys ...string) error
	DeleteAll() error

	// Write overrides values without returning an error unless one of keys already exists and is a value (ErrNotKey)
	Write(value string, keys ...string) error
	WriteAll(metadata map[string]interface{}) error

	// Dir
	Dir() string
}

var (
	ErrNoKeys   = xerrors.New("no keys provided")
	ErrNotValue = xerrors.New("not a value")
	ErrNotKey   = xerrors.New("not a key")
	ErrNotExist = xerrors.New("does not exist")
)
