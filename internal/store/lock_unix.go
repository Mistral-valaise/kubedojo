//go:build !windows

package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// Lock marks a data directory as in use by a server. A second server on the
// same SQLite file would corrupt the sessions both believe they own, and a
// restore under a running server would replace the database below it.
//
// The lock is an advisory lock of the operating system on a file in the
// directory: it ends with the process, also when the process is killed. On a
// file system that has no such locks (some network file systems) Lock
// succeeds without one, and held is false.
//
// A directory that does not exist yet is created, for its owner only: the
// lock is the first file a server writes.
func Lock(dir string) (release func(), held bool, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "kubedojo.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, ErrLocked
		}
		// No locks here. Better to run without one than not at all.
		return func() {}, false, nil
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, true, nil
}
