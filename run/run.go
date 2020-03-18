package run

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/cnb"
)

func Run(pf *packfile.Packfile) error {
	if len(os.Args) == 0 {
		return errors.New("command name missing")
	}
	command := os.Args[0]
	ctxDir := filepath.Dir(filepath.Dir(command))
	switch filepath.Base(command) {
	case "detect":
		if len(os.Args) != 3 {
			return errors.New("detect requires two arguments")
		}
		if err := cnb.Detect(pf, ctxDir, os.Args[1], os.Args[2]); err != nil {
			return err
		}
	case "build":
		if len(os.Args) != 4 {
			return errors.New("build requires three arguments")
		}
		if err := cnb.Build(pf, ctxDir, os.Args[1], os.Args[2], os.Args[3]); err != nil {
			return err
		}
	}
	return nil
}
