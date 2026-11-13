package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogLayout(t *testing.T) {
	for _, tc := range []struct {
		level slog.Level
		want  string
	}{
		{slog.LevelDebug, "DEBUG gdyn  "},
		{slog.LevelInfo, "INFO  gdyn  "},
		{slog.LevelWarn, "WARN  gdyn  "},
		{slog.LevelError, "ERROR gdyn  "},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			var output bytes.Buffer
			logger := newLogger(&output, true).With("domain", "foobar.com")
			record := slog.NewRecord(time.Date(2026, 11, 13, 12, 25, 8, 0, time.FixedZone("CET", 3600)), tc.level, "record unchanged", 0)
			record.AddAttrs(slog.String("record", "@"), slog.String("ip", "109.210.33.249"))
			if err := logger.Handler().Handle(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			want := "2026-11-13 12:25:08 " + tc.want + "record unchanged domain=foobar.com record=@ ip=109.210.33.249\n"
			if output.String() != want {
				t.Fatalf("got %q; want %q", &output, want)
			}
		})
	}
}

func TestLogEscapesMultilineEvents(t *testing.T) {
	var output bytes.Buffer
	logger := newLogger(&output, false).With("domain", "example.com").WithGroup("dns").With("record", "@")
	logger.Error("update\nfailed\r", "error", errors.New("first error\nsecond error"))
	line := output.String()
	if strings.Count(line, "\n") != 1 || strings.Contains(line, "\r") {
		t.Fatalf("event spans multiple lines: %q", line)
	}
	for _, want := range []string{`update\nfailed\r`, "domain=example.com", "dns.record=@", `dns.error="first error\nsecond error"`} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %q", want, line)
		}
	}
}
