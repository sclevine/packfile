package layers

import (
	"os"

	"github.com/sclevine/packfile"
	"github.com/sclevine/packfile/link"
	"github.com/sclevine/packfile/sync"
)

type Detect struct {
	link.Streamer
	link.Share
	*sync.Kernel
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

func (l *Detect) Backward(_ []link.Layer) {}

func (l *Detect) Forward(_ []link.Layer) {}

func (l *Detect) Links() (links []sync.Link, forTest bool) {
	return nil, false
}

func (l *Detect) Test() (exists, matched bool, err error) {
	return false, false, nil
}

func (l *Detect) Run() error {
	if err := writeLayerMetadata(l.Metadata, l.Layer); err != nil {
		return err
	}
	if l.RequireRunner == nil {
		return nil
	}
	env := packfile.NewEnvMap(os.Environ())
	md := newMetadataMap(l.Metadata)

	env["APP"] = l.AppDir
	return l.RequireRunner.Require(l.Streamer, env, md)
}

func (l *Detect) Skip() error { return nil }
