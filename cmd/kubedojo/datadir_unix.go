//go:build !windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// privateFiles are the files of the data directory that only their owner may
// read, as names and as patterns: the database with what SQLite keeps next
// to it, the settings, the lock, the backups `kubedojo backup` writes there
// by default, and what `kubedojo restore` keeps of the database it replaced.
var privateFiles = []string{
	"kubedojo.db", "kubedojo.db-wal", "kubedojo.db-shm", "settings.json", "kubedojo.lock",
	"kubedojo-*.db", "*.before-restore-*",
}

// secureDataDir creates the data directory if it does not exist and takes
// the access of everybody but its owner away: from the directory (0700) and
// from the files in it that hold progress, session keys and settings (0600).
// A directory of an older release was created readable for everyone.
//
// It does not descend: <data>/observability is mounted into the Grafana
// container, whose user must be able to read it.
//
// Where the mode cannot be changed, it stays: on a Kubernetes volume with an
// fsGroup the directory belongs to root, and access is the cluster's matter.
func secureDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := chmod(dir, 0o700); err != nil {
		return err
	}
	for _, pattern := range privateFiles {
		files, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return err
		}
		for _, f := range files {
			// Not through a link, and no directory that happens to match.
			if fi, err := os.Lstat(f); err != nil || !fi.Mode().IsRegular() {
				continue
			}
			if err := chmod(f, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// chmod is os.Chmod that leaves alone what it may not change or what is gone.
func chmod(path string, mode fs.FileMode) error {
	err := os.Chmod(path, mode)
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
