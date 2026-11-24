//go:build windows

package main

import "os"

// secureDataDir creates the data directory if it does not exist. Windows has
// no file modes: the directory is below the user's profile, whose access
// control list keeps other users out.
func secureDataDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}
