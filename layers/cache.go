package layers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/sync"
)

type Cache struct {
	Streamer
	LinkShare
	Cache  *packfile.Cache
	Shell  string
	AppDir string
	CtxDir string
}

func (l *Cache) info() linkerInfo {
	return linkerInfo{
		name:  l.Cache.Name,
		share: &l.LinkShare,
	}
}

func (l *Cache) locks(target linker) bool {
	for _, link := range target.info().links {
		if link.Name == l.Cache.Name {
			return true
		}
	}
	return false
}

func (l *Cache) backward(_ []linker, _ []*sync.Layer) {}

func (l *Cache) forward(_ []linker, _ []*sync.Layer) {}

func (l *Cache) Links() (links []sync.Link, forTest bool) {
	return nil, false
}

func (l *Cache) Test() (exists, matched bool) {
	cacheTOMLPath := l.LayerDir + ".toml"
	cacheTOML, err := readLayerTOML(cacheTOMLPath)
	if err != nil {
		l.Err = err
		return false, false
	}
	oldDigest := cacheTOML.Metadata.CodeDigest
	newDigest := l.digest()
	cacheTOML = layerTOML{Cache: true}
	cacheTOML.Metadata.CodeDigest = newDigest
	if err := writeTOML(cacheTOML, cacheTOMLPath); err != nil {
		l.Err = err
		return false, false
	}
	if _, err := os.Stat(l.LayerDir); xerrors.Is(err, os.ErrNotExist) {
		return false, false
	} else if err != nil {
		l.Err = err
		return false, false
	}
	if oldDigest != newDigest {
		return false, false
	}
	return true, true
}

func (l *Cache) Run() {
	if l.Err != nil {
		return
	}
	w, _ := l.Streamer.Writers()
	fmt.Fprintf(w, "Setting up cache '%s'.\n", l.Cache.Name)
	if err := os.RemoveAll(l.LayerDir); err != nil {
		l.Err = err
		return
	}

	env := os.Environ()
	env = append(env, "APP="+l.AppDir, "CACHE="+l.LayerDir)

	if err := os.MkdirAll(l.LayerDir, 0777); err != nil {
		l.Err = err
		return
	}
	if l.Cache.Setup == nil {
		return
	}
	cmd, c, err := execCmd(&l.Cache.Setup.Exec, l.CtxDir, l.Shell)
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

func (l *Cache) Skip() {}

func (l *Cache) digest() string {
	hash := sha256.New()
	writeField(hash, "cache")
	if l.Cache.Setup != nil {
		writeField(hash, l.Cache.Setup.Shell, l.Cache.Setup.Inline)
		writeFile(hash, l.Cache.Setup.Path)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
