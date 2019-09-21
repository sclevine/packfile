package layer

import (
	"io"
	"sync"

	"golang.org/x/xerrors"
)

var ErrNotNeeded = xerrors.New("not needed")
var ErrExists = xerrors.New("exists")
var ErrEmptyExec = xerrors.New("empty exec")

var emptyLayerExec = newLayerExec(nil)

type LinkResult struct {
	Link
	Result
}

type LayerFunc func(out, err io.Writer, res []LinkResult) (Result, error)

type layerExec struct {
	f   LayerFunc
	wg  *sync.WaitGroup
	res Result
	err error
}

func newLayerExec(f LayerFunc) *layerExec {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &layerExec{f: f, wg: wg}
}

func (l *layerExec) run(out, err io.Writer, res []LinkResult) (Result, error) {
	if l.f == nil {
		return Result{}, ErrEmptyExec
	}
	l.res, l.err = l.f(out, err, res)
	l.wg.Done()
	return l.res, l.err
}

func (l *layerExec) skip(err error) {
	if l.f == nil {
		return
	}
	l.err = err
	l.wg.Done()
}

func (l *layerExec) wait() (Result, error) {
	if l.f == nil {
		return Result{}, ErrEmptyExec
	}
	l.wg.Wait()
	return l.res, l.err
}
