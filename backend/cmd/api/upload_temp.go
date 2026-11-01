package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	uploadTempPrefix   = "anby-upload-"
	uploadTempLockName = ".owner"
	uploadInitLockName = ".anby-upload-init.lock"
)

// Each process holds its directory lock for its entire lifetime. A shared
// upload volume can therefore reclaim crashed processes without age guesses
// or deleting another API's active multipart files.
func prepareUploadTemp(parent string) (string, func(), error) {
	initLock, err := os.OpenFile(filepath.Join(parent, uploadInitLockName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", nil, err
	}
	defer initLock.Close()
	if err := syscall.Flock(int(initLock.Fd()), syscall.LOCK_EX); err != nil {
		return "", nil, err
	}
	defer syscall.Flock(int(initLock.Fd()), syscall.LOCK_UN)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), uploadTempPrefix) {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		owner, err := os.OpenFile(filepath.Join(path, uploadTempLockName), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return "", nil, err
		}
		lockErr := syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			owner.Close()
			continue
		}
		if lockErr != nil {
			owner.Close()
			return "", nil, lockErr
		}
		removeErr := os.RemoveAll(path)
		owner.Close()
		if removeErr != nil {
			return "", nil, removeErr
		}
	}
	path, err := os.MkdirTemp(parent, uploadTempPrefix)
	if err != nil {
		return "", nil, err
	}
	owner, err := os.OpenFile(filepath.Join(path, uploadTempLockName), os.O_CREATE|os.O_RDWR, 0600)
	if err == nil {
		err = syscall.Flock(int(owner.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	}
	if err != nil {
		if owner != nil {
			owner.Close()
		}
		os.RemoveAll(path)
		return "", nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(path)
		_ = owner.Close()
	}
	return path, cleanup, nil
}
