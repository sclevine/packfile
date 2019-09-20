package layer

import (
	"io"
	"sync"

	"golang.org/x/xerrors"
)

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

func (l *layerExec) run(out, err io.Writer, res []LinkResult) {
	if l == nil {
		return
	}
	l.res, l.err = l.f(out, err, res)
	l.wg.Done()
}

func (l *layerExec) skip(err error) {
	if l == nil {
		return
	}
	l.err = err
	l.wg.Done()
}

func (l *layerExec) error() error {
	if l == nil {
		return nil
	}
	return l.err
}

func (l *layerExec) hasError(err error) bool {
	if l == nil {
		return err == nil
	}
	return xerrors.Is(l.err, err)
}

func (l *layerExec) wait() Result {
	if l == nil {
		return Result{}
	}
	l.wg.Wait()
	return l.res
}
