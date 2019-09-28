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

type LayerFunc func(lrs []LinkResult) (Result, error)

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

func (l *Exec) Run(lrs []LinkResult) (Result, error) {
	if l.f == nil {
		return Result{}, ErrEmpty
	}
	defer l.wg.Done()
	l.res, l.err = l.f(lrs)
	return l.res, l.err
}

func (l *Exec) Skip(err error) {
	if l.f == nil {
		return
	}
	defer l.wg.Done()
	l.err = err
}

func (l *Exec) Set(res Result, err error) {
	if l.f == nil {
		return
	}
	defer l.wg.Done()
	l.res, l.err = res, err
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

type Streamer struct {
	out, err *BufferPipe
}

func NewStreamer() *Streamer {
	return &Streamer{
		out: NewBufferPipe(),
		err: NewBufferPipe(),
	}
}

func (l *Streamer) Writers() (out, err io.Writer) {
	return l.out, l.err
}

func (l *Streamer) Stream(out, err io.Writer) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		io.Copy(out, l.out)
		wg.Done()
	}()
	go func() {
		io.Copy(err, l.err)
		wg.Done()
	}()
	wg.Wait()
}

func (l *Streamer) Close() {
	defer l.err.Close()
	defer l.out.Close()
	l.out.Flush()
	l.err.Flush()
}