package packfile

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"golang.org/x/xerrors"
)

func (pf *Packfile) Detect(platformDir, planPath string) error {
	if err := loadEnv(filepath.Join(platformDir, "env")); err != nil {
		return err
	}
	appDir, err := os.Getwd()
	if err != nil {
		return err
	}
	var status wgList
	for _, l := range pf.Layers {
		status = status.For(l.Name)
		go l.detect(status, appDir)
	}
	status.StreamAll(os.Stdout, os.Stderr)
	status.WaitAll()

	return nil
}

func (l *Layer) detect(status wgList, appDir string) {
	env := os.Environ()
	env = append(env, "APP="+appDir)

	for _, r := range l.Detect.Require {
		result, ok := status.Wait(r.Name)
		if !ok {
			status.Done(wgResult{Err: xerrors.Errorf("require '%s' not found", r.Name)})
			return
		}
		if result.Err != nil {
			status.Done(wgResult{Err: xerrors.Errorf("require '%s' failed: %w", r.Name, result.Err)})
			return
		}
		if r.VersionEnv != "" {
			if version, err := ioutil.ReadFile(filepath.Join(result.Path, "version")); err == nil {
				env = append(env, r.VersionEnv+"="+string(version))
			} else if !os.IsNotExist(err) {
				status.Done(wgResult{Err: err})
				return
			}
		}
		if r.MetadataEnv != "" {
			env = append(env, r.MetadataEnv+"="+result.Path)
		}
	}
	dir, err := ioutil.TempDir("", "packfile."+l.Name)
	if err != nil {
		status.Done(wgResult{Err: err})
		return
	}
	env = append(env, "MD="+dir)
	cmd := exec.Command(l.Detect.Path)
	cmd.Dir = appDir
	cmd.Env = env
	cmd.Stdout = status.Out()
	cmd.Stderr = status.Err()
	if err := cmd.Run(); err != nil {
		status.Done(wgResult{Err: err})
		return
	}

	status.Done(wgResult{Path: dir})
}

type wgList []wgItem

type wgItem struct {
	name   string
	wg     *sync.WaitGroup
	result *wgResult
	stdout *BufferPipe
	stderr *BufferPipe
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

type wgResult struct {
	Err  error
	Path string
}

func (w wgList) For(name string) wgList {
	wg := &sync.WaitGroup{}
	wg.Add(1)

	return append(w, wgItem{
		name:   name,
		wg:     wg,
		result: &wgResult{},
		stdout: NewBufferPipe(),
		stderr: NewBufferPipe(),
	})
}

func (w wgList) Wait(name string) (wgResult, bool) {
	if len(w) == 0 {
		return wgResult{}, false
	}
	for _, item := range w[:len(w)-1] {
		if item.name == name {
			item.wg.Wait()
			return *item.result, true
		}
	}
	return wgResult{}, false
}

func (w wgList) WaitAll() () { // should return build plan
	for _, item := range w {
		item.wg.Wait()
	}
	for _, item := range w {
		item.result.Path
	}
}

func (w wgList) StreamAll(stdout, stderr io.Writer) {
	for _, item := range w {
		wg := &sync.WaitGroup{}
		wg.Add(2)
		go func() {
			io.Copy(stdout, item.stdout)
			wg.Done()
		}()
		go func() {
			io.Copy(stderr, item.stderr)
			wg.Done()
		}()
		wg.Wait()
	}
}

func (w wgList) Out() io.Writer {
	if len(w) == 0 {
		return nil
	}
	return w[len(w)-1].stdout
}

func (w wgList) Err() io.Writer {
	if len(w) == 0 {
		return nil
	}
	return w[len(w)-1].stderr
}

func (w wgList) Done(result wgResult) {
	if len(w) == 0 {
		return
	}
	item := w[len(w)-1]
	*item.result = result
	item.wg.Done()
	item.stdout.Flush()
	item.stderr.Flush()
	item.stdout.Close()
	item.stderr.Close()
}

func loadEnv(dir string) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		if !fi.IsDir() {
			v, err := ioutil.ReadFile(filepath.Join(dir, fi.Name()))
			if err != nil {
				return err
			}
			if err := os.Setenv(fi.Name(), string(v)); err != nil {
				return err
			}
		}
	}
	return nil
}
