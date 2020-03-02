package sync

import (
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
	t      LinkType
	testWG *sync.WaitGroup
	runWG  *sync.WaitGroup
	c      chan<- Event
	done   chan struct{}
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
	Run()
	Skip()
	Test() (exists, matched bool)
	Links() (links []Link, forTest bool)
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

type LinkType int

const (
	LinkNone LinkType = iota
	LinkRequire
	LinkContent
	LinkVersion
	LinkSerial
)

func (l *Layer) Link(t LinkType) Link {
	return Link{
		t:      t,
		testWG: l.testWG,
		runWG:  l.runWG,
		c:      l.c,
		done:   l.done,
	}
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
	go func() {
		select {
		case link.c <- ev:
		case <-link.done:
			l.lock.release()
		}
	}()
}

func (l *Layer) try(links []Link) {
	defer close(l.done)
	defer l.runWG.Done()

	for _, link := range links {
		if link.t == LinkRequire {
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
					if link.t == LinkRequire || link.t == LinkSerial {
						link.runWG.Wait()
					}
				}
				l.runner.Run()
			} else {
				l.runner.Skip()
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
		if link.t == LinkRequire {
			l.send(link, EventRequire)
		}
	}
	l.lock.release()
	for _, link := range links {
		if link.t == LinkRequire || link.t == LinkSerial {
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
		switch link.t {
		case LinkRequire:
			l.send(link, EventRequire)
		case LinkContent:
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
			switch link.t {
			case LinkRequire:
				l.send(link, EventRequire)
			case LinkContent, LinkVersion:
				l.send(link, EventChange)
			}
		}
		l.exists = true
		l.change = true
	}
}