package sync

import (
	"sync"

	"golang.org/x/xerrors"
)

type Lock struct {
	n   int
	c   chan struct{}
	mut sync.Mutex
}

func NewLock() *Lock {
	return &Lock{
		c: make(chan struct{}),
	}
}

func (l *Lock) Add(n int) {
	l.mut.Lock()
	l.n += n
	l.mut.Unlock()
}

func (l *Lock) claim() {
	l.Add(1)
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

type LinkType int

const (
	LinkNone LinkType = iota
	LinkRequire
	LinkContent
	LinkVersion
	LinkSerial
)

type Link struct {
	t    LinkType
	node *Kernel
}

type Node interface {
	Run() error
	Skip() error
	Test() (exists, matched bool, err error)
	Links() (links []Link, forTest bool)

	kernel() *Kernel
}

func RunNode(node Node) {
	node.kernel().run(node)
}

func WaitForNode(node Node) {
	node.kernel().wait()
}

func NodeLink(node Node, t LinkType) Link {
	return node.kernel().link(t)
}

func NodeError(node Node) error {
	return node.kernel().err
}

type Kernel struct {
	err     error
	name    string
	matched bool
	exists  bool
	change  bool
	testWG  *sync.WaitGroup
	runWG   *sync.WaitGroup
	c       chan Event
	done    chan struct{}
	lock    *Lock
}

func NewKernel(name string, lock *Lock) *Kernel {
	testWG := &sync.WaitGroup{}
	testWG.Add(1)
	runWG := &sync.WaitGroup{}
	runWG.Add(1)
	return &Kernel{
		name:   name,
		testWG: testWG,
		runWG:  runWG,
		c:      make(chan Event),
		done:   make(chan struct{}),
		lock:   lock,
	}
}

func (k *Kernel) kernel() *Kernel {
	return k
}

func (k *Kernel) link(t LinkType) Link {
	return Link{
		t:    t,
		node: k,
	}
}

func (k *Kernel) wait() {
	<-k.done
}

func (k *Kernel) run(node Node) {
	links, forTest := node.Links()
	if forTest {
		k.tryAfter(node, links)
	} else {
		k.try(node, links)
	}
}

func (k *Kernel) send(link Link, ev Event) {
	k.lock.claim()
	go func() {
		select {
		case link.node.c <- ev:
		case <-link.node.done:
			k.lock.release()
		}
	}()
}

func (k *Kernel) try(node Node, links []Link) {
	defer close(k.done)
	defer k.runWG.Done()

	for _, link := range links {
		if link.t == LinkRequire {
			link.node.testWG.Wait()
			if k.err == nil && link.node.err != nil { // TODO: how do I know Err isn't being written to? double check!
				k.err = xerrors.Errorf("link '%s' failed: %w", link.node.name, link.node.err)
			}
		}
	}

	if k.err == nil {
		k.exists, k.matched, k.err = node.Test()
	}
	k.testWG.Done()

	k.init(links)
	k.lock.release()

	for {
		select {
		case ev := <-k.c:
			k.trigger(links, ev)
			k.lock.release()
		case <-k.lock.wait():
			if k.err != nil {
				return
			}
			if k.change {
				for _, link := range links {
					if link.t == LinkRequire || link.t == LinkSerial {
						link.node.runWG.Wait()
					}
					if link.t == LinkRequire && link.node.err != nil {
						k.err = xerrors.Errorf("link '%s' failed: %w", link.node.name, link.node.err)
						return
					}
				}
				k.err = node.Run()
			} else {
				k.err = node.Skip()
			}
			return
		}
	}
}

// NOTE: EventChange/EventVersion delivery not guaranteed without reverse EventRequire link
func (k *Kernel) tryAfter(node Node, links []Link) {
	defer close(k.done)
	defer k.runWG.Done()

	for _, link := range links {
		if link.t == LinkRequire {
			k.send(link, EventRequire)
		}
	}
	k.lock.release()
	for _, link := range links {
		if link.t == LinkRequire || link.t == LinkSerial {
			link.node.runWG.Wait()
		}
		if link.t == LinkRequire && k.err == nil && link.node.err != nil {
			k.err = xerrors.Errorf("link '%s' failed: %w", link.node.name, link.node.err)
		}
	}

	if k.err == nil {
		k.exists, k.matched, k.err = node.Test()
	}
	k.testWG.Done()

	k.init(links)

	for {
		select {
		case ev := <-k.c:
			k.trigger(links, ev)
			k.lock.release()
		case <-k.lock.wait():
			if k.err != nil {
				return
			}
			if k.change {
				k.err = node.Run()
			} else {
				k.err = node.Skip()
			}
			return
		}
	}
}

func (k *Kernel) trigger(links []Link, ev Event) {
	if ev == EventRequire && k.exists ||
		ev == EventChange && k.change {
		return
	}
	for _, link := range links {
		switch link.t {
		case LinkRequire:
			k.send(link, EventRequire)
		case LinkContent:
			k.send(link, EventChange)
		}
	}
	k.exists = true
	k.change = true
}

func (k *Kernel) init(links []Link) {
	if !k.matched {
		if k.exists {
			panic("invalid state: present but non-matching")
		}
		for _, link := range links {
			switch link.t {
			case LinkRequire:
				k.send(link, EventRequire)
			case LinkContent, LinkVersion:
				k.send(link, EventChange)
			}
		}
		k.exists = true
		k.change = true
	}
}
