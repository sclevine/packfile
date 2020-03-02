package sync

import (
	"io"
	rsync "sync"
)

func NewPTeeReader(r io.Reader, w io.Writer) *PTeeReader {
	bufs := make(chan []byte, 10)
	t := &PTeeReader{r: r, w: w, bufs: bufs}
	go func() {
		for b := range bufs {
			if t.err != nil {
				continue
			}
			n, err := t.w.Write(b)
			t.n += n
			if err != nil {
				t.err = err
			}
			t.wg.Done()
		}
	}()
	return t
}

type PTeeReader struct {
	r    io.Reader
	w    io.Writer
	bufs chan<- []byte
	wg   rsync.WaitGroup
	n    int
	err  error
}

func (t *PTeeReader) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	if n > 0 {
		t.wg.Add(1)
		t.bufs <- p[:n]
	}
	return
}

func (t *PTeeReader) Sync() (n int, err error) {
	t.wg.Wait()
	close(t.bufs)
	return t.n, t.err
}
