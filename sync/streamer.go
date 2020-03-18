package sync
//
//import (
//	"bufio"
//	"io"
//
//	"golang.org/x/sync/errgroup"
//)
//
//type BufferPipe struct {
//	*bufio.Writer
//	io.Reader
//	io.Closer
//}
//
//func NewBufferPipe() *BufferPipe {
//	r, wc := io.Pipe()
//	return &BufferPipe{
//		Writer: bufio.NewWriter(wc),
//		Reader: r,
//		Closer: wc,
//	}
//}
//
//type Streamer struct {
//	out, err *BufferPipe
//}
//
//// FIXME: out/err order not guaranteed, fix this and drop useless Flush
//func NewStreamer() *Streamer {
//	return &Streamer{
//		out: NewBufferPipe(),
//		err: NewBufferPipe(),
//	}
//}
//
//func (l *Streamer) Out() io.Writer {
//	return l.out
//}
//
//func (l *Streamer) Err() io.Writer {
//	return l.err
//}
//
//func (l *Streamer) Stream(out, err io.Writer) error {
//	g := errgroup.Group{}
//	g.Go(func() error {
//		_, err := io.Copy(out, l.out)
//		return err
//	})
//	g.Go(func() error {
//		_, err := io.Copy(err, l.err)
//		return err
//	})
//	return g.Wait()
//}
//
//func (l *Streamer) Flush() error {
//	oErr := l.out.Flush()
//	eErr := l.err.Flush()
//	if oErr != nil {
//		return oErr
//	}
//	return eErr
//}
//
//func (l *Streamer) Close() error {
//	if err := l.Flush(); err != nil {
//		return err
//	}
//	oErr := l.out.Close()
//	eErr := l.err.Close()
//	if oErr != nil {
//		return oErr
//	}
//	return eErr
//}
