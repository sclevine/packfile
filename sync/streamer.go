package sync

import (
	"errors"
	"io"
)

type writeFd byte

const (
	stdout writeFd = 1
	stderr writeFd = 2

	defaultSize = 4096
)

type writeMsg struct {
	fd  writeFd
	buf []byte
}

type Streamer struct {
	n      int
	w, r   chan writeMsg
	done   chan struct{}
	msgBuf []writeMsg
}

func NewStreamer() *Streamer {
	s := &Streamer{
		w:    make(chan writeMsg),
		r:    make(chan writeMsg),
		done: make(chan struct{}),
	}
	go func() {
		var r chan writeMsg
		w := s.w
		for {
			select {
			case msg := <-w:
				s.n += len(msg.buf)
				s.msgBuf = append(s.msgBuf, msg)
				r = s.r
				if s.n >= defaultSize {
					w = nil
				}
			case r <- s.next():
				s.n -= len(s.msgBuf[0].buf)
				s.msgBuf = s.msgBuf[1:]
				if len(s.msgBuf) == 0 {
					r = nil
				}
				if s.n < defaultSize {
					w = s.w
				}
			case <-s.done:
				for _, msg := range s.msgBuf {
					s.r <- msg
				}
				close(s.r)
				close(s.w)
				return
			}
		}
	}()
	return s
}

func (s *Streamer) next() writeMsg {
	if len(s.msgBuf) == 0 {
		return writeMsg{}
	}
	return s.msgBuf[0]
}


func (s *Streamer) Close() error {
	close(s.done)
	return nil
}

func (s *Streamer) Stream(out, err io.Writer) error {
	var e error
	for msg := range s.r {
		if e != nil {
			continue
		}
		switch msg.fd {
		case stdout:
			_, e = out.Write(msg.buf)
		case stderr:
			_, e = err.Write(msg.buf)
		}
	}
	return e
}

func (s Streamer) Stdout() io.Writer {
	return &Writer{fd: stdout, c: s.w}
}

func (s Streamer) Stderr() io.Writer {
	return &Writer{fd: stderr, c: s.w}
}

type Writer struct {
	fd writeFd
	c  chan<- writeMsg
}

func (w *Writer) Write(p []byte) (n int, err error) {
	out := make([]byte, len(p))
	n = copy(out, p)
	defer func() {
		if r := recover(); r != nil {
			n = 0
			err = errors.New("streamer closed")
		}
	}()
	w.c <- writeMsg{fd: w.fd, buf: out}
	return n, nil
}
