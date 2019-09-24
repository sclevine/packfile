package layer

import (
	"io"
	"sync"

	"golang.org/x/xerrors"
)

var ErrNotNeeded = xerrors.New("not needed")
var ErrExists = xerrors.New("exists")
var ErrEmpty = xerrors.New("empty")

var emptyLayerExec = newLayerExec(nil, nil, nil)

type LinkResult struct {
	Link
	Result
}

type LayerFunc func(res []LinkResult, out, err io.Writer) (Result, error)

type layerExec struct {
	f          LayerFunc
	wg         *sync.WaitGroup
	res        Result
	err        error
	outw, errw io.Writer
}

func newLayerExec(f LayerFunc, out, err io.Writer) *layerExec {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &layerExec{
		f: f, wg: wg,
		outw: out,
		errw: err,
	}
}

func (l *layerExec) run(res []LinkResult) (Result, error) {
	if l.f == nil {
		return Result{}, ErrEmpty
	}
	l.res, l.err = l.f(res, l.outw, l.errw)
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
		return Result{}, ErrEmpty
	}
	l.wg.Wait()
	return l.res, l.err
}

type layerBool struct {
	wg  *sync.WaitGroup
	res bool
}

func newLayerBool() *layerBool {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &layerBool{wg: wg}
}

func (l *layerBool) set(v bool) {
	l.res = v
	l.wg.Done()
}

func (l *layerBool) wait() bool {
	l.wg.Wait()
	return l.res
}
