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
	links  []Link
	wg     *sync.WaitGroup
	result *Result
	stdout *BufferPipe
	stderr *BufferPipe
}

type Link struct {
	Name        string `toml:"name"`
	Write       bool   `toml:"write"`
	Cache       bool   `toml:"cache"`
	PathEnv     string `toml:"path-as"`
	VersionEnv  string `toml:"version-as"`
	MetadataEnv string `toml:"metadata-as"`
}

type Result struct {
	Err          error
	LayerPath    string
	MetadataPath string
}

type FinalResult struct {
	Name string
	Result
}

func (m Mux) For(name string, links ...Link) Mux {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	return append(m, layer{
		name:   name,
		links:  links,
		wg:     wg,
		result: &Result{},
		stdout: newBufferPipe(),
		stderr: newBufferPipe(),
	})
}

func (l *layer) writes(name string) bool {
	for _, link := range l.links {
		if link.Name == name && link.Write {
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

func (m Mux) Wait(fn func(Link, Result) error) error {
	if len(m) == 0 {
		return nil
	}
	for _, link := range m[len(m)-1].links {
		i := m.find(link.Name)
		if i < 0 {
			return xerrors.Errorf("'%s' not found", link.Name)
		}
		m[i].wg.Wait()
		for _, after := range m[i+1 : len(m)-1] {
			if after.writes(m[i].name) {
				after.wg.Wait()
			}
		}
		if err := fn(link, *m[i].result); err != nil {
			return err
		}
	}
	return nil
}

func (m Mux) WaitAll() []FinalResult {
	var out []FinalResult
	for _, layer := range m {
		layer.wg.Wait()
		out = append(out, FinalResult{layer.name, *layer.result})
	}
	return out
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
	layer := m[len(m)-1]
	*layer.result = result
	layer.wg.Done()
	layer.stdout.Flush()
	layer.stderr.Flush()
	layer.stdout.Close()
	layer.stderr.Close()
}

type BufferPipe struct {
	*bufio.Writer
	io.Reader
	io.Closer
}

func newBufferPipe() *BufferPipe {
	r, wc := io.Pipe()
	return &BufferPipe{
		Writer: bufio.NewWriter(wc),
		Reader: r,
		Closer: wc,
	}
}
