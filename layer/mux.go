package layer

import (
	"bufio"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

type Mux []layer

type layer struct {
	name     string
	links    []Link
	runExec  *layerExec
	testExec *layerExec
	stdout   *BufferPipe
	stderr   *BufferPipe
}

func (l *layer) skip(err error) {
	l.testExec.skip(err)
	l.runExec.skip(err)
}

var ErrNotNeeded = xerrors.New("not needed")
var ErrExists = xerrors.New("exists")

func (l *layer) run(prev, next []layer) {
	defer func() {
		l.stdout.Flush()
		l.stderr.Flush()
		l.stdout.Close()
		l.stderr.Close()
	}()

	// PROBLEM: test can't read linked test metadata unless ForTest applies to whole link (but that might cause deadlock?)
	var afterTest []*layerExec
	for _, link := range l.links {
		i := find(link.Name, prev)
		if i < 0 {
			l.skip(xerrors.Errorf("'%s' not found", link.Name))
			return
		}
		prev[i].testExec.wait()
		if link.ForTest {
			prev[i].runExec.wait()
		} else {
			afterTest = append(afterTest, prev[i].runExec)
		}
	}
	l.testExec.run(l.stdout, l.stderr)
	for _, re := range afterTest {
		re.wait()
	}

	if l.testExec.hasError(nil) || (l.testExec.hasError(ErrNotNeeded) && required(l.name, next)) {
		l.runExec.run(l.stdout, l.stderr)
	} else if !l.testExec.hasError(ErrNotNeeded) && !l.testExec.hasError(ErrExists)  {
		l.runExec.skip(xerrors.Errorf("error during test: %w", l.testExec.error()))
	}
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

type FinalResult struct {
	Name string
	Result
}

func (m Mux) Layer(name string, test, run LayerFunc, links ...Link) Mux {
	var testExec, runExec *layerExec
	if test != nil {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		testExec = &layerExec{
			f:  test,
			wg: wg,
		}
	}
	if run != nil {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		runExec = &layerExec{
			f:  run,
			wg: wg,
		}
	}

	return append(m, layer{
		name:     name,
		links:    links,
		testExec: testExec,
		runExec:  runExec,
		stdout:   newBufferPipe(),
		stderr:   newBufferPipe(),
	})
}

func (m Mux) Cache(name string, setup LayerFunc) Mux {
	return m.Layer(name, nil, setup)
}

func find(name string, layers []layer) int {
	for i := range layers {
		if layers[i].name == name {
			return i
		}
	}
	return -1
}

func required(name string, layers []layer) bool {
	for _, layer := range layers {
		for _, link := range layer.links {
			if link.Name == name {
				if link.ForTest {
					return true
				}
				layer.testExec.wait()
				if layer.testExec.hasError(nil) {
					return true
				}
			}
		}
	}
	return false
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

func (m Mux) Run() {
	for i, layer := range m {
		go layer.run(m[:i], m[i+1:])
	}
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
