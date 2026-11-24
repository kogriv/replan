//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
)

var (
	kernel32DLL      = syscall.NewLazyDLL("kernel32.dll")
	lockFileExProc   = kernel32DLL.NewProc("LockFileEx")
	unlockFileExProc = kernel32DLL.NewProc("UnlockFileEx")
)

func tryProjectFileLock(file *os.File, shared bool) (bool, error) {
	flags := uintptr(lockfileFailImmediately)
	if !shared {
		flags |= lockfileExclusiveLock
	}
	var overlapped syscall.Overlapped
	result, _, callErr := lockFileExProc.Call(
		file.Fd(),
		flags,
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result != 0 {
		return true, nil
	}
	if errors.Is(callErr, errorLockViolation) {
		return false, nil
	}
	return false, callErr
}

func unlockProjectFile(file *os.File) error {
	var overlapped syscall.Overlapped
	result, _, callErr := unlockFileExProc.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if result != 0 {
		return nil
	}
	return callErr
}

// A valid project already has project.yaml. It only has to stay in place
// until the first writer creates the persistent lock, before any mutation.
func acquireProjectBootstrapLock(path string, timeout time.Duration, shared bool) (*projectWriteLock, error) {
	path = filepath.Join(path, "project.yaml")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return acquireProjectFileLock(path, file, err, timeout, shared)
}
