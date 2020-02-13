package layers

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/sync"
)

type Detect struct {
	Streamer
	LinkShare
	Layer  *packfile.Layer
	Shell  string
	AppDir string
}

func (l *Detect) info() layerInfo {
	return layerInfo{
		name:  l.Layer.Name,
		share: &l.LinkShare,
	}
}

func (l *Detect) locks(_ linker) bool {
	return false
}

func (l *Detect) backward(_ []linker, _ []*sync.Layer) {}

func (l *Detect) forward(_ []linker, _ []*sync.Layer) {}

func (l *Detect) Links() (links []sync.Link, forTest bool) {
	return nil, false
}

func (l *Detect) Test() (exists, matched bool) {
	return false, false
}

func (l *Detect) Run() {
	if err := writeMetadata(l.MetadataDir, l.Layer.Version, l.Layer.Metadata); err != nil {
		l.Err = err
		return
	}
	if l.Layer.Require == nil {
		return
	}

	env := os.Environ()
	env = append(env, "APP="+l.AppDir, "MD="+l.MetadataDir)
	cmd, c, err := execCmd(&l.Layer.Require.Exec, l.Shell)
	if err != nil {
		l.Err = err
		return
	}
	defer c.Close()
	cmd.Dir = l.AppDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = l.Writers()
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				l.Err = CodeError(status.ExitStatus())
				return
			}
		}
		l.Err = err
		return
	}
}
