package clog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogger_Levels(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil) // disable stderr for testing
	l.SetLevel(LevelDebug)

	l.Debug("debug message")
	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")

	output := buf.String()

	// Check all levels are present
	if !strings.Contains(output, "[DEBUG] debug message") {
		t.Errorf("expected debug message in output, got: %s", output)
	}
	if !strings.Contains(output, "[INFO] info message") {
		t.Errorf("expected info message in output, got: %s", output)
	}
	if !strings.Contains(output, "[WARN] warn message") {
		t.Errorf("expected warn message in output, got: %s", output)
	}
	if !strings.Contains(output, "[ERROR] error message") {
		t.Errorf("expected error message in output, got: %s", output)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil)
	l.SetLevel(LevelWarn) // Only warn and above

	l.Debug("debug message")
	l.Info("info message")
	l.Warn("warn message")
	l.Error("error message")

	output := buf.String()

	// Debug and Info should be filtered out
	if strings.Contains(output, "debug message") {
		t.Errorf("debug message should be filtered, got: %s", output)
	}
	if strings.Contains(output, "info message") {
		t.Errorf("info message should be filtered, got: %s", output)
	}
	// Warn and Error should be present
	if !strings.Contains(output, "[WARN] warn message") {
		t.Errorf("expected warn message in output, got: %s", output)
	}
	if !strings.Contains(output, "[ERROR] error message") {
		t.Errorf("expected error message in output, got: %s", output)
	}
}

func TestLogger_DaemonMode(t *testing.T) {
	var fileBuf, errBuf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&fileBuf)
	l.SetErrOutput(&errBuf)
	l.SetLevel(LevelDebug)

	// CLI mode: warn/error go to both file and stderr
	l.Warn("cli warning")
	l.Error("cli error")

	if !strings.Contains(fileBuf.String(), "cli warning") {
		t.Errorf("expected warning in file output")
	}
	if !strings.Contains(errBuf.String(), "cli warning") {
		t.Errorf("expected warning in stderr output")
	}

	// Clear buffers
	fileBuf.Reset()
	errBuf.Reset()

	// Daemon mode: only file output
	l.SetDaemonMode(true)
	l.Warn("daemon warning")
	l.Error("daemon error")

	if !strings.Contains(fileBuf.String(), "daemon warning") {
		t.Errorf("expected warning in file output")
	}
	if strings.Contains(errBuf.String(), "daemon warning") {
		t.Errorf("daemon mode should not write to stderr")
	}
}

func TestLogger_FormatWithArgs(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil)
	l.SetLevel(LevelDebug)

	l.Info("count: %d, name: %s", 42, "test")

	output := buf.String()
	if !strings.Contains(output, "count: 42, name: test") {
		t.Errorf("expected formatted message, got: %s", output)
	}
}

func TestLogger_TimestampFormat(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger()
	l.SetFileOutput(&buf)
	l.SetErrOutput(nil)

	l.Info("test")

	output := buf.String()
	// Should have RFC3339 timestamp (e.g., 2024-01-15T14:32:05Z)
	if !strings.Contains(output, "T") || !strings.Contains(output, "Z") {
		t.Errorf("expected RFC3339 timestamp, got: %s", output)
	}
}

func TestOpenLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.log")

	f, err := OpenLogFile(path)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v", err)
	}
	defer f.Close()

	// Write something
	_, err = f.WriteString("test log entry\n")
	if err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}

	// Close and reopen to test append mode
	f.Close()
	f, err = OpenLogFile(path)
	if err != nil {
		t.Fatalf("OpenLogFile() reopen error = %v", err)
	}
	defer f.Close()

	_, err = f.WriteString("second entry\n")
	if err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	f.Close()

	// Read back
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(content), "test log entry") {
		t.Errorf("expected first entry in file")
	}
	if !strings.Contains(string(content), "second entry") {
		t.Errorf("expected second entry in file")
	}
}

func TestDefaultLogPath(t *testing.T) {
	path := DefaultLogPath()
	if !strings.Contains(path, "cloister") {
		t.Errorf("expected 'cloister' in path, got: %s", path)
	}
	if !strings.HasSuffix(path, ".log") {
		t.Errorf("expected .log suffix, got: %s", path)
	}
}

func TestCloisterLogPath(t *testing.T) {
	path := CloisterLogPath("my-project-main")
	if !strings.Contains(path, "cloisters") {
		t.Errorf("expected 'cloisters' in path, got: %s", path)
	}
	if !strings.HasSuffix(path, "my-project-main.log") {
		t.Errorf("expected my-project-main.log suffix, got: %s", path)
	}
}
