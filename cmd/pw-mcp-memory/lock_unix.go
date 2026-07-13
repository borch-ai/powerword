//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

type FileLock struct {
	path string
	f    *os.File
}

func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

func (l *FileLock) Lock() error {
	lockPath := l.path + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0750); err != nil {
		return err
	}

	//nolint:gosec // G304: lock path is pre-validated by main dbPath config
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.f = f

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		l.f = nil
		return err
	}
	return nil
}

func (l *FileLock) Unlock() {
	if l.f != nil {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		_ = l.f.Close()
		_ = os.Remove(l.path + ".lock")
	}
}
