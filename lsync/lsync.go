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
	OldLink
	Result
	Preserved   bool
	SameVersion bool // FIXME: doesn't belong here!
}

type OldLink struct {
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

type Layer struct {
	runner    Runner
	matched   bool
	exists    bool
	change    bool
	testWG    *sync.WaitGroup
	runWG     *sync.WaitGroup
	c         chan Event
	done      chan struct{}
	lock      *Lock
}

type Link struct {
	Require bool
	Content bool
	Version bool
	c       chan<- Event
	done    chan struct{}
}

type Runner interface {
	Test() (exists, matched bool)
	Run()
	Links() (links []Link, forTest bool)
}

func NewLayer(lock *Lock, runner Runner) *Layer {
	testWG := &sync.WaitGroup{}
	testWG.Add(1)
	runWG := &sync.WaitGroup{}
	runWG.Add(1)
	return &Layer{
		runner:    runner,
		testWG:    testWG,
		runWG:     runWG,
		c:         make(chan Event),
		done:      make(chan struct{}),
		lock:      lock,
	}
}

func (l *Layer) send(link Link, ev Event) {
	l.lock.claim()
	select {
	case link.c <- ev:
	case <-link.done:
		l.lock.release()
	}
}

func (l *Layer) Run() {
	links, forTest := l.runner.Links()
	if forTest {
		l.runWithLinks(links)
	} else {
		l.run(links)
	}
}

func (l *Layer) run(links []Link) {
	defer close(l.done)

	for _, link := range links {
		if link.Require {
			l.testWG.Wait()
		}
	}

	l.exists, l.matched = l.runner.Test()
	l.testWG.Done()

	l.init(links)
	l.lock.release()

	for {
		select {
		case ev := <-l.c:
			l.trigger(links, ev)
			l.lock.release()
		case <-l.lock.wait():
			if l.change {
				for _, link := range links {
					if link.Require {
						l.runWG.Wait()
					}
				}
				l.runner.Run()
			}
			l.runWG.Done()
		}
	}
}

func (l *Layer) runWithLinks(links []Link) {
	defer close(l.done)

	for _, link := range links {
		if link.Require {
			l.send(link, EventRequire)
		}
	}
	l.lock.release()
	for _, link := range links {
		if link.Require {
			l.runWG.Wait()
		}
	}

	l.exists, l.matched = l.runner.Test()
	l.testWG.Done()

	l.init(links)

	for {
		select {
		case ev := <-l.c:
			l.trigger(links, ev)
			l.lock.release()
		case <-l.lock.wait():
			if l.change {
				l.runner.Run()
			}
			l.runWG.Done()
		}
	}
}

// r.present = version-matching layer is present

func (l *Layer) trigger(links []Link, ev Event) {
	if ev == EventRequire && l.exists ||
		ev == EventChange && l.change {
		return
	}
	for _, link := range links {
		if link.Require {
			l.send(link, EventRequire)
		}
		if link.Content {
			l.send(link, EventChange)
		}
	}
	l.exists = true
	l.change = true
}

func (l *Layer) init(links []Link) {
	if !l.matched {
		if l.exists {
			panic("invalid state: present but non-matching")
		}
		for _, link := range links {
			if link.Require {
				l.send(link, EventRequire)
			}
			if link.Content || link.Version {
				l.send(link, EventChange)
			}
		}
		l.exists = true
		l.change = true
	}
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
