package layer

import (
	"bufio"
	"io"
	"sync"

	"golang.org/x/xerrors"
)

type Mux []layer

type layer struct {
	name      string
	links     []Link
	runExec   *layerExec
	testExec  *layerExec
	mustBuild *layerBool
	stdout    *BufferPipe
	stderr    *BufferPipe
}

func (l *layer) skip(err error) {
	l.mustBuild.set(false)
	l.testExec.skip(err)
	l.runExec.skip(err)
}

func (l *layer) run(prev, next []layer) {
	defer l.close()

	linkLayers, err := findAll(l.links, prev)
	if err != nil {
		l.skip(err)
		return
	}
	var results []LinkResult
	for i, ll := range linkLayers {
		result, err := ll.testExec.wait()
		if err != nil {
			// FIXME: should we propagate non-ErrNotNeeded, non-ErrExists errors forward?
		}
		if l.links[i].ForTest {
			result, err = ll.runExec.wait()
			if err != nil {
				// FIXME: should we propagate run errors forward?
			}
		}
		results = append(results, LinkResult{Link: l.links[i], Result: result})
	}

	_, err = l.testExec.run(results)
	if err == nil || err == ErrEmpty || (xerrors.Is(err, ErrNotNeeded) && required(l.name, next)) {
		l.mustBuild.set(true)
		var results []LinkResult
		for i, ll := range linkLayers {
			result, err := ll.runExec.wait()
			if err != nil {
				// FIXME: should we propagate run errors forward?
			}
			results = append(results, LinkResult{Link: l.links[i], Result: result})
		}
		_, err := l.runExec.run(results)
		if err != nil {
			// FIXME: what about this error?
		}
	} else if !xerrors.Is(err, ErrNotNeeded) && !xerrors.Is(err, ErrExists) {
		l.mustBuild.set(false)
		l.runExec.skip(xerrors.Errorf("error during test: %w", err))
	} else {
		l.mustBuild.set(false)
		l.runExec.skip(nil)
	}
}

func (l *layer) close() {
	l.stdout.Flush()
	l.stderr.Flush()
	l.stdout.Close()
	l.stderr.Close()
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
	outw, errw := newBufferPipe(), newBufferPipe()
	reqWG := &sync.WaitGroup{}
	reqWG.Add(1)
	return append(m, layer{
		name:      name,
		links:     links,
		testExec:  newLayerExec(test, outw, errw),
		runExec:   newLayerExec(run, outw, errw),
		mustBuild: newLayerBool(),
		stdout:    outw,
		stderr:    errw,
	})
}

func (m Mux) Cache(name string, setup LayerFunc) Mux {
	return m.Layer(name, nil, setup)
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

func (m Mux) WaitAll() []FinalResult {
	var out []FinalResult
	for _, layer := range m {
		result, err := layer.runExec.wait()

		out = append(out, FinalResult{layer.name, result})
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
