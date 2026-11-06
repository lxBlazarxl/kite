package agent

import (
	"io"
	"os"
	"sync"
)

type rotatingFile struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

// NewRotatingFile appends to path; when a write would push it past
// maxBytes, path is renamed to path+".1" (replacing any older copy) and a
// fresh file is started. Keeps a long-running service's log bounded.
func NewRotatingFile(path string, maxBytes int64) (io.WriteCloser, error) {
	r := &rotatingFile{path: path, max: maxBytes}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		r.f.Close()
		os.Remove(r.path + ".1")
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			return 0, err
		}
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}
