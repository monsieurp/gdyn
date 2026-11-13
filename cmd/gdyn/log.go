package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

type logOutput struct {
	mu     sync.Mutex
	fields bytes.Buffer
	writer io.Writer
}

type logHandler struct {
	fields slog.Handler
	output *logOutput
}

func newLogger(writer io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	output := &logOutput{writer: writer}
	fields := slog.NewTextHandler(&output.fields, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && (attr.Key == slog.TimeKey || attr.Key == slog.LevelKey || attr.Key == slog.MessageKey) {
				return slog.Attr{}
			}
			return attr
		},
	})
	return slog.New(&logHandler{fields: fields, output: output})
}

func (handler *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.fields.Enabled(ctx, level)
}

func (handler *logHandler) Handle(ctx context.Context, record slog.Record) error {
	// Derived loggers share the buffer and writer. Keep each event in one write.
	handler.output.mu.Lock()
	defer handler.output.mu.Unlock()
	handler.output.fields.Reset()
	if err := handler.fields.Handle(ctx, record); err != nil {
		return err
	}
	message := strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(record.Message)
	line := fmt.Sprintf("%s %-5s gdyn  %s", record.Time.Format("2006-01-02 15:04:05"), record.Level, message)
	if fields := strings.TrimSuffix(handler.output.fields.String(), "\n"); fields != "" {
		line += " " + fields
	}
	_, err := io.WriteString(handler.output.writer, line+"\n")
	return err
}

func (handler *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{fields: handler.fields.WithAttrs(attrs), output: handler.output}
}

func (handler *logHandler) WithGroup(name string) slog.Handler {
	return &logHandler{fields: handler.fields.WithGroup(name), output: handler.output}
}
