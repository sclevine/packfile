package lsync

import (
	"bufio"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

var ErrEmpty = xerrors.New("empty")

var EmptyExec = NewExec(nil)

type LinkResult struct {
	Link
	Result
}

type Link struct {
	Name         string `toml:"name"`
	PathEnv      string `toml:"path-as"`
	VersionEnv   string `toml:"version-as"`
	MetadataEnv  string `toml:"metadata-as"`
	ForTest      bool   `toml:"for-test"`
	LinkContents bool   `toml:"link-contents"`
	LinkVersion  bool   `toml:"link-version"`
}

type Result struct {
	LayerPath    string
	MetadataPath string
}

type LayerFunc func(res []LinkResult) (Result, error)

type Exec struct {
	f   LayerFunc
	wg  *sync.WaitGroup
	res Result
	err error
}

func NewExec(f LayerFunc) *Exec {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &Exec{f: f, wg: wg}
}

func (l *Exec) Run(res []LinkResult) (Result, error) {
	if l.f == nil {
		return Result{}, ErrEmpty
	}
	defer l.wg.Done()
	l.res, l.err = l.f(res)
	return l.res, l.err
}

func (l *Exec) Skip(err error) {
	if l.f == nil {
		return
	}
	l.err = err
	l.wg.Done()
}

func (l *Exec) Wait() (Result, error) {
	if l.f == nil {
		return Result{}, ErrEmpty
	}
	l.wg.Wait()
	return l.res, l.err
}

type Bool struct {
	wg  *sync.WaitGroup
	res bool
}

func NewBool() *Bool {
	wg := &sync.WaitGroup{}
	wg.Add(1)
	return &Bool{wg: wg}
}

func (l *Bool) Set(v bool) {
	l.res = v
	l.wg.Done()
}

func (l *Bool) Wait() bool {
	l.wg.Wait()
	return l.res
}

type BufferPipe struct {
	*bufio.Writer
	io.Reader
	io.Closer
}

func NewBufferPipe() *BufferPipe {
	r, wc := io.Pipe()
	return &BufferPipe{
		Writer: bufio.NewWriter(wc),
		Reader: r,
		Closer: wc,
	}
}
