package layer

import (
	"bufio"
	"io"
	"sync"
)

type Mux []layer

type layer struct {
	name   string
	wg     *sync.WaitGroup
	result *Result
	stdout *BufferPipe
	stderr *BufferPipe
}

type Result struct {
	Err  error
	Path string
}

func (m Mux) For(name string) Mux {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	return append(m, layer{
		name:   name,
		wg:     wg,
		result: &Result{},
		stdout: NewBufferPipe(),
		stderr: NewBufferPipe(),
	})
}

func (m Mux) Wait(name string) (Result, bool) {
	if len(m) == 0 {
		return Result{}, false
	}
	for _, layer := range m[:len(m)-1] {
		if layer.name == name {
			layer.wg.Wait()
			return *layer.result, true
		}
	}
	return Result{}, false
}

//func (m Mux) NewWait(fn ) ([]Result, error) {
//	if len(m) == 0 {
//		return nil, nil
//	}
//	for i, layer := range m[:len(m)-1] {
//		if layer.name == name {
//			layer.wg.Wait()
//
//			for _, r := range m[i+1:len(m)-1] {
//
//			}
//
//			return *layer.result, true
//		}
//	}
//	return Result{}, false
//}

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
