//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
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

	// LOCKFILE_EXCLUSIVE_LOCK = 2
	err = windows.LockFileEx(windows.Handle(f.Fd()), 2, 0, 1, 0, &windows.Overlapped{})
	if err != nil {
		f.Close()
		return err
	}
	return nil
}

func (l *FileLock) Unlock() {
	if l.f != nil {
		_ = windows.UnlockFileEx(windows.Handle(l.f.Fd()), 0, 1, 0, &windows.Overlapped{})
		_ = l.f.Close()
		_ = os.Remove(l.path + ".lock")
	}
}
