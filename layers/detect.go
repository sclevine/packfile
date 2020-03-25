package layers

import (
	"os"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/sync"
)

type Detect struct {
	Streamer
	link.Share
	Layer         *packfile.Layer
	RequireRunner packfile.RequireRunner
	AppDir        string
}

func (l *Detect) Info() link.Info {
	return link.Info{
		Name:  l.Layer.Name,
		Share: &l.Share,
	}
}

func (l *Detect) Locks(_ link.Layer) bool {
	return false
}

func (l *Detect) Backward(_ []link.Layer, _ []*sync.Layer) {}

func (l *Detect) Forward(_ []link.Layer, _ []*sync.Layer) {}

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
