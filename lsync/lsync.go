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
	Preserved   bool
	SameVersion bool // FIXME: doesn't belong here!
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

func (r Result) LayerTOML() string {
	return r.LayerPath + ".toml"
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

type lock struct {
	n   int
	c   chan struct{}
	mut sync.Mutex
}

func (l *lock) claim() {
	l.mut.Lock()
	l.n++
	l.mut.Unlock()
}

// panics if called more times than claim
func (l *lock) release() {
	l.mut.Lock()
	l.n--
	if l.n <= 0 {
		close(l.c)
	}
	l.mut.Unlock()
}

func (l *lock) wait() <-chan struct{} {
	return l.c
}

func newLock() lock {
	return lock{
		c: make(chan struct{}),
	}
}

type Event int

const (
	EventRequire = iota
	EventChange
)

type Resolver struct {
	matched bool
	present bool
	changed bool
	c       chan Event
	done    chan struct{}
}

type Linc struct {
	Require  bool
	Contents bool
	Version  bool
	c        chan<- Event
	done     chan struct{}
}

func (l *Linc) send(ev Event) {
	select {
	case l.c <- ev:
	case <-l.done:
	}
}

func NewResolver(matched, present bool) *Resolver {
	return &Resolver{
		matched: matched,
		present: present,
		c:       make(chan Event),
		done:    make(chan struct{}),
	}
}

func (r *Resolver) Wait(links []Linc) (present, changed bool) {
	defer close(r.done)
	if !r.matched {
		if r.present {
			panic("invalid state: present but non-matching")
		}
		for _, l := range links {
			if l.Require {
				l.send(EventRequire)
			}
			if l.Contents || l.Version {
				l.send(EventChange)
			}
		}
		r.present = true
		r.changed = true
	}
	gl := newLock()
	select {
	case ev := <-r.c:
		r.trigger(links, ev)
	case <-gl.wait():
		return
	}
	return r.present, r.changed
}

// r.present = version-matching layer is present

func (r *Resolver) trigger(links []Linc, ev Event) {
	switch ev {
	case EventRequire:
		if r.present {
			return
		}
		for _, l := range links {
			if l.Require {
				l.send(EventRequire)
			}
			if l.Contents {
				l.send(EventChange)
			}
		}
		r.present = true
		r.changed = true
	case EventChange:
		if r.changed {
			return
		}
		for _, l := range links {
			if l.Require {
				l.send(EventRequire)
			}
			if l.Contents {
				l.send(EventChange)
			}
		}
		r.present = true
		r.changed = true
	}
}



//type Resolver struct {
//	status chan bool
//	done   chan struct{}
//	wg     *sync.WaitGroup
//	change bool
//}
//
//func NewResolver() *Resolver {
//	return &Resolver{
//		status: make(chan bool),
//		done:   make(chan struct{}),
//		wg:     &sync.WaitGroup{},
//	}
//}
//// need to send signal to link when only link change could change own behavior
//func (r *Resolver) Wait(links []Resolver) (change, ok bool) {
//	defer close(r.done)
//	changes := make(chan struct{})
//	earlyChanges := changes
//
//	go func() {
//		defer close(changes)
//		for _, l := range links {
//			l.wg.Wait()
//			if l.change {
//				changes <- struct{}{}
//				return
//			}
//		}
//		earlyChanges = nil
//	}()
//	select {
//	case _, change := <-earlyChanges:
//		return change, true
//	case ok := <-r.status:
//		_, change := <-changes
//		return change, ok
//	}
//}
//
//
//func (r *Resolver) WaitOld(links []Resolver) (change, ok bool) {
//	defer close(r.done)
//	changes := make(chan struct{})
//	earlyChanges := changes
//
//	go func() {
//		defer close(changes)
//		for _, l := range links {
//			l.wg.Wait()
//			if l.change {
//				changes <- struct{}{}
//				return
//			}
//		}
//		earlyChanges = nil
//	}()
//	select {
//	case _, change := <-earlyChanges:
//		return change, true
//	case pass := <-r.status:
//		if !pass {
//			return false, false
//		}
//		Trigger(links)
//		_, change := <-changes
//		return change, true
//	}
//}
//
//func (r *Resolver) WaitOld2(links []Resolver) (change, ok bool) {
//	defer close(r.done)
//	changes := make(chan struct{})
//
//	go func() {
//		defer close(changes)
//		for _, l := range links {
//			l.wg.Wait()
//			if l.change {
//				changes <- struct{}{}
//				return
//			}
//		}
//	}()
//	select {
//	case _, change = <-changes:
//		ok = true
//	case ok = <-r.status:
//	}
//	if !ok {
//		return false, false
//	}
//	Trigger(links)
//	_, change := <-changes
//	return change, true
//}
//
//func (r *Resolver) Change(v bool) {
//	r.change = v
//	r.wg.Done()
//}
//
//func Trigger(links []Resolver) {
//	for _, l := range links {
//		select {
//		case l.status <- true:
//		case <-l.done:
//		}
//	}
//}

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
