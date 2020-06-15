package sync

import (
	"io"
	rsync "sync"
)

func NewPTeeReader(r io.Reader, w io.Writer) *PTeeReader {
	bufs := make(chan []byte, 10)
	t := &PTeeReader{r: r, bufs: bufs}
	go func() {
		for b := range bufs {
			if t.err != nil {
				t.wg.Done()
				continue
			}
			n, err := w.Write(b)
			t.n += int64(n)
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
	bufs chan<- []byte
	wg   rsync.WaitGroup
	n    int64
	err  error
}

func (t *PTeeReader) Read(p []byte) (n int, err error) {
	n, err = t.r.Read(p)
	if n > 0 {
		t.wg.Add(1)
		t.bufs <- append([]byte(nil), p[:n]...)
	}
	return
}

func (t *PTeeReader) Sync() (n int64, err error) {
	t.wg.Wait()
	close(t.bufs)
	return t.n, t.err
}
