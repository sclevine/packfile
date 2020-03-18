package layers

import (
	"crypto/sha256"
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/sync"
)

type Cache struct {
	Streamer
	LinkShare
	Cache       *packfile.Cache
	SetupRunner packfile.SetupRunner
	AppDir      string
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
	if err := os.RemoveAll(l.LayerDir); err != nil {
		l.Err = err
		return
	}
	if err := os.MkdirAll(l.LayerDir, 0777); err != nil {
		l.Err = err
		return
	}
	if l.SetupRunner == nil {
		return
	}
	fmt.Fprintf(l.Stdout(), "Setting up cache '%s'.\n", l.Cache.Name)
	env := packfile.NewEnvMap(os.Environ())
	env["APP"] = l.AppDir
	env["CACHE"] = l.LayerDir
	if err := l.SetupRunner.Setup(l.Streamer, env); err != nil {
		l.Err = err
		return
	}
	fmt.Fprintf(l.Stdout(), "Setup cache '%s'.\n", l.Cache.Name)
}

func (l *Cache) Skip() {
	if l.Err != nil {
		return
	}
	fmt.Fprintf(l.Stdout(), "Using existing cache '%s'.\n", l.Cache.Name)
}

func (l *Cache) digest() string {
	hash := sha256.New()
	writeField(hash, "cache")
	if l.SetupRunner != nil {
		writeField(hash, l.SetupRunner.Version())
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
