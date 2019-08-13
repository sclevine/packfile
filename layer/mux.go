package layer

import (
	"bufio"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

type Mux []layer

type layer struct {
	name   string
	reqs   []Require
	wg     *sync.WaitGroup
	result *Result
	stdout *BufferPipe
	stderr *BufferPipe
}

type Require struct {
	Name        string
	Write       bool
	VersionEnv  string
	MetadataEnv string
}

type Result struct {
	Err  error
	Path string
}

func (m Mux) For(name string, reqs []Require) Mux {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	return append(m, layer{
		name:   name,
		reqs:   reqs,
		wg:     wg,
		result: &Result{},
		stdout: NewBufferPipe(),
		stderr: NewBufferPipe(),
	})
}

func (l *layer) writes(name string) bool {
	for _, req := range l.reqs {
		if req.Name == name && req.Write {
			return true
		}
	}
	return false
}

func (m Mux) find(name string) int {
	if len(m) == 0 {
		return -1
	}
	for i := range m[:len(m)-1] {
		if m[i].name == name {
			return i
		}
	}
	return -1
}

func (m Mux) Wait(fn func(req Require, res Result) error) error {
	if len(m) == 0 {
		return nil
	}
	for _, req := range m[len(m)-1].reqs {
		i := m.find(req.Name)
		if i < 0 {
			return xerrors.Errorf("require '%s' not found", req.Name)
		}
		m[i].wg.Wait()
		for _, after := range m[i+1 : len(m)-1] {
			if after.writes(m[i].name) {
				after.wg.Wait()
			}
		}
		if err := fn(req, *m[i].result); err != nil {
			return err
		}
	}
	return nil
}

func (m Mux) WaitAll() { // should return build plan
	for _, layer := range m {
		layer.wg.Wait()
	}
	//for _, item := range c {
	//	item.result.Path
	//}
}

func (m Mux) StreamAll(stdout, stderr io.Writer) {
	for _, layer := range m {
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			io.Copy(stdout, layer.stdout)
			wg.Done()
		}()
		go func() {
			io.Copy(stderr, layer.stderr)
			wg.Done()
		}()
		wg.Wait()
	}
}

func (m Mux) Out() io.Writer {
	if len(m) == 0 {
		return nil
	}
	return m[len(m)-1].stdout
}

func (m Mux) Err() io.Writer {
	if len(m) == 0 {
		return nil
	}
	return m[len(m)-1].stderr
}

func (m Mux) Done(result Result) {
	if len(m) == 0 {
		return
	}
	item := m[len(m)-1]
	*item.result = result
	item.wg.Done()
	item.stdout.Flush()
	item.stderr.Flush()
	item.stdout.Close()
	item.stderr.Close()
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
