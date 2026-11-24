//go:build !windows

package main

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func tryProjectFileLock(file *os.File, shared bool) (bool, error) {
	mode := syscall.LOCK_EX
	if shared {
		mode = syscall.LOCK_SH
	}
	err := syscall.Flock(int(file.Fd()), mode|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return false, err
}

func unlockProjectFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}

func acquireProjectBootstrapLock(path string, timeout time.Duration, shared bool) (*projectWriteLock, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return acquireProjectFileLock(path, file, err, timeout, shared)
}
