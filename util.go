package packfile

import (
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

func IsFail(err error) bool {
	if e, ok := err.(DetectError); ok {
		return e == 100
	}
	return false
}

func IsError(err error) bool {
	if e, ok := err.(DetectError); ok {
		return e != 100
	}
	return false
}

func writeMetadata(path, version string, metadata map[string]string) error {
	for k, v := range metadata {
		if err := ioutil.WriteFile(filepath.Join(path, k), []byte(v), 0666); err != nil {
			return err
		}
	}
	if version == "" {
		return nil
	}
	return ioutil.WriteFile(filepath.Join(path, "version"), []byte(version), 0666)
}

// NOTE: implements UNIX exec-style shebang parsing for shell
func execCmd(e *Exec, shell string) (*exec.Cmd, io.Closer, error) {
	if e.Inline != "" && e.Path != "" {
		return nil, nil, xerrors.New("both inline and path specified")
	}
	if e.Shell != "" {
		shell = e.Shell
	}
	parts := strings.SplitN(shell, " ", 2)
	if len(parts) == 0 {
		return nil, nil, xerrors.New("missing shell")
	}
	var args []string
	if len(parts) > 1 {
		args = append(args, parts[1])
	}
	if e.Inline != "" {
		f, err := ioutil.TempFile("", "packfile.")
		if err != nil {
			return nil, nil, err
		}
		if _, err := f.WriteString(e.Inline); err != nil {
			return nil, nil, err
		}
		return exec.Command(shell, append(args, f.Name())...), f, nil
	}

	if e.Path == "" {
		return nil, nil, xerrors.New("missing executable")
	}

	return exec.Command(shell, append(args, e.Path)...), nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
