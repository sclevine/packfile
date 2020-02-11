package cnb

import (
	"os"

	"github.com/BurntSushi/toml"
)

func writeTOML(lt interface{}, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(lt)
}

