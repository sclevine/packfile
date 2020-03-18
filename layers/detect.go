package layers

import (
	"os"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/sync"
)

type Detect struct {
	Streamer
	LinkShare
	Layer         *packfile.Layer
	RequireRunner packfile.RequireRunner
	AppDir        string
}

func (l *Detect) info() linkerInfo {
	return linkerInfo{
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
	if err := writeLayerMetadata(l.Metadata, l.Layer); err != nil {
		l.Err = err
		return
	}
	if l.RequireRunner == nil {
		return
	}
	env := packfile.NewEnvMap(os.Environ())
	md := newMetadataMap(l.Metadata)

	env["APP"] = l.AppDir
	if err := l.RequireRunner.Require(l.Streamer, env, md); err != nil {
		l.Err = err
		return
	}
}

func (l *Detect) Skip() {}
