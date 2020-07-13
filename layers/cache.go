package layers

import (
	"crypto/sha256"
	"fmt"
	"os"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/sync"
)

type Cache struct {
	link.Streamer
	link.Share
	*sync.Kernel
	Cache       *packfile.Cache
	SetupRunner packfile.SetupRunner
	AppDir      string
}

func (l *Cache) Info() link.Info {
	return link.Info{
		Name:  l.Cache.Name,
		Share: &l.Share,
	}
}

func (l *Cache) Locks(target link.Layer) bool {
	for _, link := range target.Info().Links {
		if link.Name == l.Cache.Name {
			return true
		}
	}
	return false
}

func (l *Cache) Backward(_ []link.Layer) {}

func (l *Cache) Forward(_ []link.Layer) {}

func (l *Cache) Links() (links []sync.Link, forTest bool) {
	return nil, false
}

func (l *Cache) Test() (exists, matched bool, err error) {
	cacheTOMLPath := l.LayerDir + ".toml"
	cacheTOML, err := readLayerTOML(cacheTOMLPath)
	if err != nil {
		return false, false, err
	}
	oldDigest := cacheTOML.Metadata.CodeDigest
	newDigest := l.digest()
	cacheTOML = layerTOML{Cache: true}
	cacheTOML.Metadata.CodeDigest = newDigest
	if err := writeTOML(cacheTOML, cacheTOMLPath); err != nil {
		return false, false, err
	}
	if _, err := os.Stat(l.LayerDir); xerrors.Is(err, os.ErrNotExist) {
		return false, false, nil
	} else if err != nil {
		return false, false, err
	}
	if oldDigest != newDigest {
		return false, false, nil
	}
	return true, true, nil
}

func (l *Cache) Run() error {
	if err := os.RemoveAll(l.LayerDir); err != nil {
		return err
	}
	if err := os.MkdirAll(l.LayerDir, 0777); err != nil {
		return err
	}
	if l.SetupRunner == nil {
		return nil
	}
	fmt.Fprintf(l.Stdout(), "Setting up cache '%s'.\n", l.Cache.Name)
	env := packfile.NewEnvMap(os.Environ())
	env["APP"] = l.AppDir
	env["CACHE"] = l.LayerDir
	if err := l.SetupRunner.Setup(l.Streamer, env); err != nil {
		return err
	}
	fmt.Fprintf(l.Stdout(), "Setup cache '%s'.\n", l.Cache.Name)
	return nil
}

func (l *Cache) Skip() error {
	fmt.Fprintf(l.Stdout(), "Using existing cache '%s'.\n", l.Cache.Name)
	return nil
}

func (l *Cache) digest() string {
	hash := sha256.New()
	writeField(hash, "cache")
	if l.SetupRunner != nil {
		writeField(hash, l.SetupRunner.Version())
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
