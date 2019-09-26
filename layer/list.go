package layer

import (
	"bufio"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

type List []layer

type layer struct {
	name      string
	links     []Link
	runExec   *layerExec
	testExec  *layerExec
	mustBuild *layerBool
	lock      *sync.Mutex
	stdout    *BufferPipe
	stderr    *BufferPipe
}

func (l *layer) skip(err error) {
	l.mustBuild.set(false)
	l.testExec.skip(err)
	l.runExec.skip(err)
}

// TODO: how does require work?
func (l *layer) run(prev, next []layer) {
	defer l.close()

	linkLayers, err := findAll(l.links, prev)
	if err != nil {
		l.skip(err)
		return
	}
	var testRes []LinkResult
	for i, ll := range linkLayers {
		result, err := ll.testExec.wait()
		if IsFail(err) {
			l.skip(xerrors.Errorf("test for link '%s' failed: %w", ll.name, err))
			return
		}
		if l.links[i].ForTest {
			result, err = ll.runExec.wait()
			if IsFail(err) {
				l.skip(xerrors.Errorf("link '%s' (needed for test) failed: %w", ll.name, err))
				return
			}
		}
		testRes = append(testRes, LinkResult{Link: l.links[i], Result: result})
	}

	_, err = l.testExec.run(testRes)
	if err != nil && err != ErrEmpty && !(xerrors.Is(err, ErrNotNeeded) && required(l.name, next)) {
		l.mustBuild.set(false)
		if IsFail(err) {
			l.runExec.skip(xerrors.Errorf("test for '%s' failed: %w", l.name, err))
			return
		}
		l.runExec.skip(nil)
		return
	}
	l.mustBuild.set(true)
	var runRes []LinkResult
	for i, ll := range linkLayers {
		result, err := ll.runExec.wait()
		if IsFail(err) {
			l.runExec.skip(xerrors.Errorf("link '%s' failed: %w", l.name, err))
			return
		}
		runRes = append(runRes, LinkResult{Link: l.links[i], Result: result})
	}
	l.runExec.run(runRes)
}

func (l *layer) close() {
	defer l.stderr.Close()
	defer l.stdout.Close()
	l.stdout.Flush()
	l.stderr.Flush()
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
	Err  error
	Result
}

func NewList() List {
	return nil
}

func (m List) Add(name string, write bool, test, run LayerFunc, links ...Link) List {
	outw, errw := newBufferPipe(), newBufferPipe()
	var lock *sync.Mutex
	if len(m) != 0 {
		lock = m[0].lock
	} else {
		lock = &sync.Mutex{}
	}
	layerLock := lock
	if !write {
		layerLock = nil
	}
	return append(m, layer{
		name:      name,
		links:     links,
		testExec:  newLayerExec(test, layerLock, outw, errw),
		runExec:   newLayerExec(run, layerLock, outw, errw),
		mustBuild: newLayerBool(),
		lock:      lock,
		stdout:    outw,
		stderr:    errw,
	})
}

func (m List) AddSimple(name string, run LayerFunc) List {
	return m.Add(name, false, nil, run)
}

func findAll(links []Link, layers []layer) ([]*layer, error) {
	var out []*layer
	for _, link := range links {
		layer := find(link.Name, layers)
		if layer == nil {
			return nil, xerrors.Errorf("'%s' not found", link.Name)
		}
		out = append(out, layer)
	}
	return out, nil
}

func find(name string, layers []layer) *layer {
	for i := range layers {
		if layers[i].name == name {
			return &layers[i]
		}
	}
	return nil
}

func required(name string, layers []layer) bool {
	for _, layer := range layers {
		for _, link := range layer.links {
			if link.Name == name {
				if link.ForTest {
					return true
				}
				return layer.mustBuild.wait()
			}
		}
	}
	return false
}

func (m List) WaitAll() []FinalResult {
	var out []FinalResult
	for _, layer := range m {
		result, err := layer.runExec.wait()
		out = append(out, FinalResult{layer.name, err, result})
	}
	return out
}

func (m List) StreamAll(stdout, stderr io.Writer) {
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

func (m List) Run() {
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
