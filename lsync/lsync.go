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

type Lock struct {
	n   int
	c   chan struct{}
	mut sync.Mutex
}

func (l *Lock) claim() {
	l.mut.Lock()
	l.n++
	l.mut.Unlock()
}

// panics if called more times than claim+n
func (l *Lock) release() {
	l.mut.Lock()
	l.n--
	if l.n <= 0 {
		close(l.c)
	}
	l.mut.Unlock()
}

func (l *Lock) wait() <-chan struct{} {
	return l.c
}

func NewLock(n int) Lock {
	return Lock{
		n: n,
		c: make(chan struct{}),
	}
}

type Event int

const (
	EventRequire = iota
	EventChange
)

type Resolver struct {
	links     []Linc
	runner    Runner
	testLinks bool
	matched   bool
	exists    bool
	change    bool
	testWG    *sync.WaitGroup
	runWG     *sync.WaitGroup
	c         chan Event
	done      chan struct{}
	lock      *Lock
}

type Linc struct {
	Require bool
	Content bool
	Version bool
	c       chan<- Event
	done    chan struct{}
}

type Runner interface {
	Test() (exists, matched bool)
	Run()
}

func NewResolver(lock *Lock, links []Linc, runner Runner, testLinks bool) *Resolver {
	testWG := &sync.WaitGroup{}
	testWG.Add(1)
	runWG := &sync.WaitGroup{}
	runWG.Add(1)
	return &Resolver{
		links:     links,
		runner:    runner,
		testLinks: testLinks,
		testWG:    testWG,
		runWG:     runWG,
		c:         make(chan Event),
		done:      make(chan struct{}),
		lock:      lock,
	}
}

func (r *Resolver) send(link Linc, ev Event) {
	r.lock.claim()
	select {
	case link.c <- ev:
	case <-link.done:
		r.lock.release()
	}
}

func (r *Resolver) RunCombined() {
	defer close(r.done)

	if r.testLinks {
		for _, l := range r.links {
			if l.Require {
				r.send(l, EventRequire)
			}
		}
		r.lock.release()
		for _, l := range r.links {
			if l.Require {
				r.runWG.Wait()
			}
		}
	} else {
		for _, l := range r.links {
			if l.Require {
				r.testWG.Wait()
			}
		}
	}

	r.exists, r.matched = r.runner.Test()
	r.testWG.Done()

	if !r.matched {
		if r.exists {
			panic("invalid state: present but non-matching")
		}
		for _, l := range r.links {
			if l.Version {
				r.send(l, EventChange)
			}
		}
		r.create()
	}

	if !r.testLinks {
		r.lock.release()
	}
	for {
		select {
		case ev := <-r.c:
			r.trigger(ev)
			r.lock.release()
		case <-r.lock.wait():
			if r.change {
				if !r.testLinks {
					for _, l := range r.links {
						if l.Require {
							r.runWG.Wait()
						}
					}
				}
				r.runner.Run()
			}
			r.runWG.Done()
		}
	}
}

func (r *Resolver) Run() {
	defer close(r.done)

	for _, l := range r.links {
		if l.Require {
			r.testWG.Wait()
		}
	}

	r.exists, r.matched = r.runner.Test()
	r.testWG.Done()

	r.init()
	r.lock.release()

	for {
		select {
		case ev := <-r.c:
			r.trigger(ev)
			r.lock.release()
		case <-r.lock.wait():
			if r.change {
				for _, l := range r.links {
					if l.Require {
						r.runWG.Wait()
					}
				}
				r.runner.Run()
			}
			r.runWG.Done()
		}
	}
}

func (r *Resolver) RunAfterLinks() {
	defer close(r.done)

	for _, l := range r.links {
		if l.Require {
			r.send(l, EventRequire)
		}
	}
	r.lock.release()
	for _, l := range r.links {
		if l.Require {
			r.runWG.Wait()
		}
	}

	r.exists, r.matched = r.runner.Test()
	r.testWG.Done()

	r.init()

	for {
		select {
		case ev := <-r.c:
			r.trigger(ev)
			r.lock.release()
		case <-r.lock.wait():
			if r.change {
				r.runner.Run()
			}
			r.runWG.Done()
		}
	}
}

// r.present = version-matching layer is present

func (r *Resolver) trigger(ev Event) {
	if ev == EventRequire && r.exists ||
		ev == EventChange && r.change {
		return
	}
	r.create()
}

func (r *Resolver) init() {
	if !r.matched {
		if r.exists {
			panic("invalid state: present but non-matching")
		}
		for _, l := range r.links {
			if l.Version {
				r.send(l, EventChange)
			}
		}
		r.create()
	}
}

func (r *Resolver) create() {
	for _, l := range r.links {
		if l.Require {
			r.send(l, EventRequire)
		}
		if l.Content {
			r.send(l, EventChange)
		}
	}
	r.exists = true
	r.change = true
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
