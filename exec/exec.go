package exec

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
)

type CodeError int

func (e CodeError) Error() string {
	return fmt.Sprintf("failed with code %d", e)
}

func IsFail(err error) bool {
	var e CodeError
	if xerrors.As(err, &e) {
		return e == 100
	}
	return false
}

func IsError(err error) bool {
	var e CodeError
	if xerrors.As(err, &e) {
		return e != 100
	}
	return false
}

type Exec struct {
	packfile.Exec
	Name   string
	CtxDir string
}

func (e *Exec) Version() string {
	hash := sha256.New()
	writeField(hash, e.Shell, e.Inline)
	writeFile(hash, e.Path)
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func writeField(out io.Writer, values ...interface{}) {
	for _, v := range values {
		fmt.Fprintln(out, v)
	}
}

func writeFile(out io.Writer, path string) {
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		fmt.Fprintln(out, f)
	}
}

func (e *Exec) Test(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata) error {
	mddir, ok := md.(interface{ Dir() string })
	if !ok {
		return xerrors.New("metadata directory not available")
	}
	env["MD"] = mddir.Dir()
	return e.run(st, env)
}

func (e *Exec) Provide(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata, deps []packfile.Dep) error {
	mddir, ok := md.(interface{ Dir() string })
	if !ok {
		return xerrors.New("metadata directory not available")
	}
	env["MD"] = mddir.Dir()

	tmpDir, err := ioutil.TempDir("", "packfile.deps."+e.Name)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	storeDir := filepath.Join(tmpDir, "store")
	if err := os.Mkdir(storeDir, 0777); err != nil {
		return err
	}
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := writeTOML(packfile.ConfigTOML{
		ContextDir:  e.CtxDir,
		StoreDir:    storeDir,
		MetadataDir: mddir.Dir(),
		Deps:        deps,
	}, configPath); err != nil {
		return err
	}
	env["PF_CONFIG_PATH"] = configPath
	return e.run(st, env)
}

func (e *Exec) Require(st packfile.Streamer, env packfile.EnvMap, md packfile.Metadata) error {
	mddir, ok := md.(interface{ Dir() string })
	if !ok {
		return xerrors.New("metadata directory not available")
	}
	env["MD"] = mddir.Dir()
	return e.run(st, env)
}

func (e *Exec) Setup(st packfile.Streamer, env packfile.EnvMap) error {
	return e.run(st, env)
}

func (e *Exec) run(st packfile.Streamer, env packfile.EnvMap) error {
	cmd, c, err := execCmd(&e.Exec, e.CtxDir)
	if err != nil {
		return err
	}
	defer c.Close()
	cmd.Dir = env["APP"]
	cmd.Env = env.Environ()
	cmd.Stdout, cmd.Stderr = st.Stdout(), st.Stderr()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return CodeError(status.ExitStatus())
			}
		}
		return err
	}
	return nil
}

// NOTE: implements UNIX exec-style shebang parsing for shell
func execCmd(e *packfile.Exec, ctxDir string) (*exec.Cmd, io.Closer, error) {
	if e.Inline != "" && e.Path != "" {
		return nil, nil, xerrors.New("both inline and path specified")
	}
	shell := e.Shell
	parts := strings.SplitN(shell, " ", 2)
	if len(parts) == 0 {
		return nil, nil, xerrors.New("missing shell")
	}
	var args []string
	if len(parts) > 1 {
		shell = parts[0]
		args = append(args, parts[1])
	}
	if e.Inline != "" {
		f, err := ioutil.TempFile("", "packfile.")
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		if _, err := f.WriteString(e.Inline); err != nil {
			return nil, nil, err
		}
		return exec.Command(shell, append(args, f.Name())...), rmCloser{f.Name()}, nil
	}

	if e.Path == "" {
		return nil, nil, xerrors.New("missing executable")
	}

	return exec.Command(shell, append(args, filepath.Join(ctxDir, e.Path))...), nopCloser{}, nil
}

type rmCloser struct{ path string }

func (c rmCloser) Close() error { return os.Remove(c.path) }

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func writeTOML(v interface{}, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(v)
}
