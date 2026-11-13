package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigLockAliasesAndIndependentConfigs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "config.toml")
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireConfigLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	alias := filepath.Join(dir, "alias.toml")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, "./config.toml", alias} {
		other, err := acquireConfigLock(name)
		if other != nil {
			other.Close()
		}
		if !errors.Is(err, errRunActive) {
			t.Errorf("path %q: got %v; want active run", name, err)
		}
	}
	secondPath := filepath.Join(dir, "second.toml")
	if err := saveConfig(secondPath, testConfig()); err != nil {
		t.Fatal(err)
	}
	second, err := acquireConfigLock(secondPath)
	if err != nil {
		t.Fatalf("independent configuration was blocked: %v", err)
	}
	defer second.Close()
	// Atomic replacement of the configuration must not bypass the lock.
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	if other, err := acquireConfigLock(path); !errors.Is(err, errRunActive) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("configuration replacement bypassed lock: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path + ".lock")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("lock file missing or wrong permissions: %v, %v", info, err)
	}
	last, err := acquireConfigLock(path)
	if err != nil {
		t.Fatalf("lock was not released on close: %v", err)
	}
	defer last.Close()
}

func TestConfigLockHelper(t *testing.T) {
	path := os.Getenv("GDYN_TEST_LOCK_PATH")
	if path == "" {
		return
	}
	lock, err := acquireConfigLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Fprintln(os.Stdout, "locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestConfigLockReleasedAfterProcessKilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestConfigLockHelper$")
	command.Env = append(os.Environ(), "GDYN_TEST_LOCK_PATH="+path)
	input, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("child failed to acquire lock: %q, %v", line, err)
	}
	if other, err := acquireConfigLock(path); !errors.Is(err, errRunActive) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("child did not exclude parent: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	lock, err := acquireConfigLock(path)
	if err != nil {
		t.Fatalf("OS did not release the killed process's lock: %v", err)
	}
	defer lock.Close()
}

func TestCLISkipsActiveRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireConfigLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	for _, fileLogging := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		args := []string{"--config", path, "--dry-run"}
		logPath := filepath.Join(t.TempDir(), "gdyn.log")
		if fileLogging {
			args = append(args, "--log", logPath)
		}
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("busy run returned %d; want 0", code)
		}
		logs := stderr.String()
		if fileLogging {
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			logs = string(data)
			if stderr.Len() != 0 {
				t.Fatalf("file logging leaked to terminal: %q", &stderr)
			}
		}
		if stdout.Len() != 0 || !strings.Contains(logs, "run already active") {
			t.Fatalf("unexpected output: stdout=%q logs=%q", &stdout, logs)
		}
	}
}

func TestCLILockFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".lock", 0700); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--config", path}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "configuration lock failed") {
		t.Fatalf("unexpected exit or error: %d, %q", code, &stderr)
	}
}
