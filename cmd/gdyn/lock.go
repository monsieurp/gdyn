package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var errRunActive = errors.New("run already active")

func acquireConfigLock(configPath string) (*os.File, error) {
	path, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve configuration path: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve configuration path: %w", err)
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open configuration lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, errRunActive
		}
		return nil, fmt.Errorf("acquire configuration lock: %w", err)
	}
	// Closing this descriptor releases the lock. Keep the file in place so
	// later runs always lock the same inode, including after a config save.
	return file, nil
}
