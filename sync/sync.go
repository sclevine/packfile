package sync

import (
	"bufio"
	"io"
	"sync"
)

type Lock struct {
	n   int
	c   chan struct{}
	mut sync.Mutex
}

func NewLock(n int) *Lock {
	return &Lock{
		n: n,
		c: make(chan struct{}),
	}
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

type Event int

const (
	EventRequire = iota
	EventChange
)

type Link struct {
	require bool
	content bool
	version bool
	cache   bool
	testWG  *sync.WaitGroup
	runWG   *sync.WaitGroup
	c       chan<- Event
	done    chan struct{}
}

type Layer struct {
	runner  Runner
	matched bool
	exists  bool
	change  bool
	testWG  *sync.WaitGroup
	runWG   *sync.WaitGroup
	c       chan Event
	done    chan struct{}
	lock    *Lock
}

type Runner interface {
	Links() (links []Link, forTest bool)
	Test() (exists, matched bool)
	Run()
}

func NewLayer(lock *Lock, runner Runner) *Layer {
	testWG := &sync.WaitGroup{}
	testWG.Add(1)
	runWG := &sync.WaitGroup{}
	runWG.Add(1)
	return &Layer{
		runner: runner,
		testWG: testWG,
		runWG:  runWG,
		c:      make(chan Event),
		done:   make(chan struct{}),
		lock:   lock,
	}
}

type LinkOption int

const (
	Require LinkOption = iota
	Content
	Version
	Cache
)

func (l LinkOption) apply(link *Link) {
	switch l {
	case Require:
		link.require = true
	case Content:
		link.content = true
	case Version:
		link.version = true
	case Cache:
		link.cache = true
	}
}

func (l *Layer) Link(opts ...LinkOption) Link {
	link := Link{
		testWG:  l.testWG,
		runWG:   l.runWG,
		c:       l.c,
		done:    l.done,
	}
	for _, opt := range opts {
		opt.apply(&link)
	}
	return link
}

func (l *Layer) Wait() {
	<-l.done
}

func (l *Layer) Run() {
	links, forTest := l.runner.Links()
	if forTest {
		l.tryAfter(links)
	} else {
		l.try(links)
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

func (l *Layer) try(links []Link) {
	defer close(l.done)
	defer l.runWG.Done()

	for _, link := range links {
		if link.require {
			link.testWG.Wait()
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
					if link.require || link.cache {
						link.runWG.Wait()
					}
				}
				l.runner.Run()
			}
			return
		}
	}
}

// NOTE: EventChange/EventVersion delivery not guaranteed without reverse EventRequire link
func (l *Layer) tryAfter(links []Link) {
	defer close(l.done)
	defer l.runWG.Done()

	for _, link := range links {
		if link.require {
			l.send(link, EventRequire)
		}
	}
	l.lock.release()
	for _, link := range links {
		if link.require || link.cache {
			link.runWG.Wait()
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
			return
		}
	}
}

func (l *Layer) trigger(links []Link, ev Event) {
	if ev == EventRequire && l.exists ||
		ev == EventChange && l.change {
		return
	}
	for _, link := range links {
		if link.require {
			l.send(link, EventRequire)
		}
		if link.content {
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
			if link.require {
				l.send(link, EventRequire)
			}
			if link.content || link.version {
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
