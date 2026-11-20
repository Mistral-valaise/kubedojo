//go:build windows

package store

import (
	"errors"
	"os"
	"path/filepath"
)

// Lock marks a data directory as in use by a server; see the Unix version.
// Here the lock is the existence of a file, which a killed server leaves
// behind: remove kubedojo.lock by hand in that case.
func Lock(dir string) (release func(), held bool, err error) {
	path := filepath.Join(dir, "kubedojo.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, false, ErrLocked
	}
	if err != nil {
		return nil, false, err
	}
	return func() {
		f.Close()
		_ = os.Remove(path)
	}, true, nil
}
