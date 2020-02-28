package metadata

type Store interface {
	Read(keys ...string) (string, error)
	ReadAll() (map[string]interface{}, error)
	Delete(keys ...string) error
	DeleteAll() error
	Write(value string, keys ...string) error
	WriteAll(metadata map[string]interface{}) error
}
