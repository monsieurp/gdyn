package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{"help", []string{"--help"}, 0, "Usage: gdyn"},
		{"configure help", []string{"configure", "--help"}, 0, "Usage: gdyn configure"},
		{"version", []string{"--version"}, 0, "gdyn dev"},
		{"unknown flag", []string{"--unknown"}, 2, "flag provided but not defined"},
		{"unexpected argument", []string{"unexpected"}, 2, "unexpected argument"},
		{"configure rejects update flag", []string{"configure", "--force"}, 2, "flag provided but not defined"},
		{"missing default configuration", nil, 1, "configuration failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tc.args, &stdout, &stderr)
			if code != tc.code || !strings.Contains(stdout.String()+stderr.String(), tc.want) {
				t.Fatalf("got exit %d, stdout=%q, stderr=%q; want exit %d containing %q", code, &stdout, &stderr, tc.code, tc.want)
			}
		})
	}
}

func TestLogFileRoutingAndAppend(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "gdyn.log")
	configPath := filepath.Join(dir, "missing.toml")
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"--config", configPath, "--log", logPath}, &stdout, &stderr); code != 1 {
			t.Fatalf("got exit %d; want 1 for missing configuration", code)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("file logging leaked to terminal: stdout=%q stderr=%q", &stdout, &stderr)
		}
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines; want two appended entries: %s", len(lines), content)
	}
	for _, line := range lines {
		checkTimestamp(t, line)
		if !strings.Contains(line, " ERROR gdyn  ") || !strings.Contains(line, "configuration failed") {
			t.Errorf("unexpected log entry: %s", line)
		}
	}
}

func TestTerminalLogsHaveTimestamps(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--config", filepath.Join(t.TempDir(), "missing.toml")}, &stdout, &stderr)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("unexpected exit or stdout: %d %q", code, &stdout)
	}
	checkTimestamp(t, strings.TrimSpace(stderr.String()))
}

func checkTimestamp(t *testing.T, line string) {
	t.Helper()
	if len(line) < 19 {
		t.Fatalf("missing timestamp: %q", line)
	}
	if _, err := time.Parse("2006-01-02 15:04:05", line[:19]); err != nil {
		t.Fatalf("invalid timestamp: %v", err)
	}
}

func TestVerboseLogging(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		var logs bytes.Buffer
		logger := newLogger(&logs, verbose)
		logger.Debug("comparison details")
		logger.Info("record unchanged")
		if strings.Contains(logs.String(), "comparison details") != verbose || !strings.Contains(logs.String(), "record unchanged") {
			t.Fatalf("unexpected logs for verbose=%v: %s", verbose, &logs)
		}
	}
}

func TestLogCannotOverwriteConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--config", path, "--log", path}, &stdout, &stderr); code != 1 {
		t.Fatalf("got exit %d; want rejection", code)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("log output changed configuration")
	}
}
