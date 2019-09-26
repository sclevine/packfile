package layer

import (
	"io"
	"sync"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile/lsync"
)

var (
	ErrNotNeeded = xerrors.New("not needed")
	ErrExists    = xerrors.New("exists")
)

func IsFail(err error) bool {
	return err != nil &&
		!xerrors.Is(err, ErrNotNeeded) &&
		!xerrors.Is(err, ErrExists)
}

type List []layer

type layer struct {
	name     string
	links    []lsync.Link
	runExec  *lsync.Exec
	testExec *lsync.Exec
	force    *lsync.Bool
	stdout   *lsync.BufferPipe
	stderr   *lsync.BufferPipe
}

func (l *layer) skip(err error) {
	l.force.Set(false)
	l.testExec.Skip(err)
	l.runExec.Skip(err)
}

func (l *layer) run(prev, next []layer) {
	defer l.close()

	linkLayers, err := findAll(l.links, prev)
	if err != nil {
		l.skip(err)
		return
	}
	var testRes []lsync.LinkResult
	for i, ll := range linkLayers {
		result, err := ll.testExec.Wait()
		if IsFail(err) {
			l.skip(xerrors.Errorf("test for link '%s' failed: %w", ll.name, err))
			return
		}
		if l.links[i].ForTest {
			result, err = ll.runExec.Wait()
			if IsFail(err) {
				l.skip(xerrors.Errorf("link '%s' (needed for test) failed: %w", ll.name, err))
				return
			}
		}
		testRes = append(testRes, lsync.LinkResult{Link: l.links[i], Result: result})
	}

	_, err = l.testExec.Run(testRes)
	// FIXME: before proceeding to run, wait for further tests to finish (i.e., wait via required)
	if err != nil && err != lsync.ErrEmpty && !(xerrors.Is(err, ErrNotNeeded) && required(l.name, next)) {
		l.force.Set(false)
		if IsFail(err) {
			l.runExec.Skip(xerrors.Errorf("test for '%s' failed: %w", l.name, err))
			return
		}
		// TODO: propagate test result forward?
		l.runExec.Skip(nil)
		return
	}
	l.force.Set(true)
	var runRes []lsync.LinkResult
	for i, ll := range linkLayers {
		result, err := ll.runExec.Wait()
		if IsFail(err) {
			l.runExec.Skip(xerrors.Errorf("link '%s' failed: %w", l.name, err))
			return
		}
		runRes = append(runRes, lsync.LinkResult{Link: l.links[i], Result: result})
	}
	l.runExec.Run(runRes)
}

func (l *layer) stream(out, err io.Writer) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		io.Copy(out, l.stdout)
		wg.Done()
	}()
	go func() {
		io.Copy(err, l.stderr)
		wg.Done()
	}()
	wg.Wait()
}

func (l *layer) close() {
	defer l.stderr.Close()
	defer l.stdout.Close()
	l.stdout.Flush()
	l.stderr.Flush()
}

type FinalResult struct {
	Name string
	Err  error
	lsync.Result
}

func NewList() List {
	return nil
}

type Layer interface {
	Links() []lsync.Link
	Run(results []lsync.LinkResult) (lsync.Result, error)
	Test(results []lsync.LinkResult) (lsync.Result, error)
	Stream(out, err io.Writer)
	Close()
}

func (m List) Add(name string, test, run lsync.LayerFunc, links ...lsync.Link) List {
	stdout, stderr := NewBufferPipe(), NewBufferPipe()
	return append(m, layer{
		name:     name,
		links:    links,
		testExec: lsync.NewExec(test),
		runExec:  lsync.NewExec(run),
		force:    lsync.NewBool(),
	})
}

func findAll(links []lsync.Link, layers []layer) ([]*layer, error) {
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
				return layer.force.Wait()
			}
		}
	}
	return false
}

func (m List) WaitAll() []FinalResult {
	var out []FinalResult
	for _, layer := range m {
		result, err := layer.runExec.Wait()
		out = append(out, FinalResult{layer.name, err, result})
	}
	return out
}

func (m List) StreamAll(stdout, stderr io.Writer) {
	for _, layer := range m {
		layer.stream(stdout, stderr)
	}
}

func (m List) Run() {
	for i, layer := range m {
		go layer.run(m[:i], m[i+1:])
	}
}