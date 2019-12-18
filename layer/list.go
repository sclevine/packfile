package layer

import (
	"io"

	"golang.org/x/xerrors"

	"github.com/sclevine/packfile/lsync"
)

var (
	ErrNotNeeded = xerrors.New("not needed")
	ErrExists    = xerrors.New("exists")
)

func IsFail(err error) bool {
	return err != nil && !IsNotChanged(err)
}

func IsNotChanged(err error) bool {
	return xerrors.Is(err, ErrNotNeeded) ||
		xerrors.Is(err, ErrExists)
}

type List []entry

type entry struct {
	Streamer
	name     string
	links    []lsync.Link
	runExec  *lsync.Exec
	testExec *lsync.Exec
	change   *lsync.Bool
}

func (e *entry) skip(err error) {
	e.change.Set(false)
	e.testExec.Skip(err)
	e.runExec.Skip(err)
}

func (e *entry) run(prev, next []entry) {
	defer e.Close()

	linkLayers, err := findAll(e.links, prev)
	if err != nil {
		e.skip(err)
		return
	}
	var testRes []lsync.LinkResult
	for i, ll := range linkLayers {
		result, err := ll.testExec.Wait()
		if IsFail(err) {
			e.skip(xerrors.Errorf("test for link '%s' failed: %w", ll.name, err))
			return
		}
		sameVersion := IsNotChanged(err)
		if e.links[i].ForTest {
			result, err = ll.runExec.Wait()
			if IsFail(err) {
				e.skip(xerrors.Errorf("link '%s' (needed for test) failed: %w", ll.name, err))
				return
			}
		}
		testRes = append(testRes, lsync.LinkResult{
			Link:        e.links[i],
			Result:      result,
			NoChange:    !ll.change.Wait(),
			SameVersion: sameVersion,
		})
	}

	result, err := e.testExec.Run(testRes)
	if err != nil && err != lsync.ErrEmpty && !(xerrors.Is(err, ErrNotNeeded) && used(e.name, next)) {
		e.change.Set(false)
		if IsFail(err) {
			e.runExec.Skip(xerrors.Errorf("test for '%s' failed: %w", e.name, err))
			return
		}
		e.runExec.Set(result, err)
		return
	}
	// before proceeding to run, wait for further tests to finish (also: used doesn't wait for all links)
	wait(e.name, next)

	e.change.Set(true) // should this go before wait? probably not because recursion?
	var runRes []lsync.LinkResult
	for i, ll := range linkLayers {
		result, err := ll.runExec.Wait()
		if IsFail(err) {
			e.runExec.Skip(xerrors.Errorf("link '%s' failed: %w", e.name, err))
			return
		}
		runRes = append(runRes, lsync.LinkResult{
			Link:        e.links[i],
			Result:      result,
			NoChange:    !ll.change.Wait(),
			SameVersion: IsNotChanged(err),
		})
	}
	e.runExec.Run(runRes)
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
	Streamer
	Name() string
	Links() []lsync.Link
	Run(results []lsync.LinkResult) (lsync.Result, error)
}

type LayerTester interface {
	Test(results []lsync.LinkResult) (lsync.Result, error)
}

type Streamer interface {
	Stream(out, err io.Writer)
	Close()
}

func (m List) Add(layer Layer) List {
	e := entry{
		Streamer: layer,
		name:     layer.Name(),
		links:    layer.Links(),
		runExec:  lsync.NewExec(layer.Run),
		change:   lsync.NewBool(),
	}
	if lt, ok := layer.(LayerTester); ok {
		e.testExec = lsync.NewExec(lt.Test)
	} else {
		e.testExec = lsync.EmptyExec
	}
	return append(m, e)
}

func findAll(links []lsync.Link, layers []entry) ([]entry, error) {
	out := make([]entry, 0, len(links))
	for _, link := range links {
		l, ok := find(link.Name, layers)
		if !ok {
			return nil, xerrors.Errorf("'%s' not found", link.Name)
		}
		out = append(out, l)
	}
	return out, nil
}

func find(name string, layers []entry) (entry, bool) {
	for i := range layers {
		if layers[i].name == name {
			return layers[i], true
		}
	}
	return entry{}, false
}

func used(name string, layers []entry) bool {
	for _, layer := range layers {
		for _, link := range layer.links {
			if link.Name == name {
				return link.ForTest || layer.change.Wait()
			}
		}
	}
	return false
}

func wait(name string, layers []entry) {
	for _, layer := range layers {
		for _, link := range layer.links {
			if link.Name == name && !link.ForTest {
				layer.change.Wait()
			}
		}
	}
}

func (m List) Wait() []FinalResult {
	var out []FinalResult
	for _, layer := range m {
		result, err := layer.runExec.Wait()
		out = append(out, FinalResult{layer.name, err, result})
	}
	return out
}

func (m List) Stream(stdout, stderr io.Writer) {
	for _, layer := range m {
		layer.Stream(stdout, stderr)
	}
}

func (m List) Run() {
	for i, layer := range m {
		go layer.run(m[:i], m[i+1:])
	}
}
