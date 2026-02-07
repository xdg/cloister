package clog

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalFunctions(t *testing.T) {
	// Save and restore global state
	defer Reset()

	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil)
	l.SetLevel(LevelDebug)
	ReplaceGlobal(l)

	Debug("debug %s", "msg")
	Info("info %s", "msg")
	Warn("warn %s", "msg")
	Error("error %s", "msg")

	output := buf.String()

	if !strings.Contains(output, "[DEBUG] debug msg") {
		t.Errorf("expected debug in output")
	}
	if !strings.Contains(output, "[INFO] info msg") {
		t.Errorf("expected info in output")
	}
	if !strings.Contains(output, "[WARN] warn msg") {
		t.Errorf("expected warn in output")
	}
	if !strings.Contains(output, "[ERROR] error msg") {
		t.Errorf("expected error in output")
	}
}

func TestConfigure(t *testing.T) {
	defer Reset()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	err := Configure(logPath, true, false)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	defer func() { _ = Close() }()

	Info("test message")

	// Read the log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("expected message in log file, got: %s", content)
	}
}

func TestDiscard(t *testing.T) {
	defer Reset()
	Discard()

	// These should not panic or produce output
	Debug("test")
	Info("test")
	Warn("test")
	Error("test")
}

func TestWriter(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil)
	l.SetLevel(LevelDebug)
	ReplaceGlobal(l)

	w := Writer(LevelInfo)
	_, err := w.Write([]byte("test from writer\n"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "[INFO] test from writer") {
		t.Errorf("expected message from writer, got: %s", output)
	}
}

func TestWriter_TrimsNewline(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil)
	l.SetLevel(LevelDebug)
	ReplaceGlobal(l)

	w := Writer(LevelInfo)
	_, _ = w.Write([]byte("message with newline\n"))

	output := buf.String()
	// Should have exactly one newline (from the log line itself)
	if strings.Count(output, "\n") != 1 {
		t.Errorf("expected single newline, got: %q", output)
	}
}

func TestConfigureEmptyPath(t *testing.T) {
	defer Reset()

	// Empty path should disable file logging
	err := Configure("", false, false)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	// Should not panic
	Info("test")
}

func TestSetFunctions(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer

	SetFileOutput(&buf)
	SetLevel(LevelDebug)
	SetDaemonMode(false)
	SetErrOutput(io.Discard)

	Debug("test debug")

	if !strings.Contains(buf.String(), "test debug") {
		t.Errorf("expected debug message after SetLevel")
	}
}
