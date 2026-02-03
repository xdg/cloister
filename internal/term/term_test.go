package term

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrint(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetOutput(&buf)

	Print("hello")

	if buf.String() != "hello" {
		t.Errorf("Print() = %q, want %q", buf.String(), "hello")
	}
}

func TestPrintf(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetOutput(&buf)

	Printf("count: %d", 42)

	if buf.String() != "count: 42" {
		t.Errorf("Printf() = %q, want %q", buf.String(), "count: 42")
	}
}

func TestPrintln(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetOutput(&buf)

	Println("hello", "world")

	want := "hello world\n"
	if buf.String() != want {
		t.Errorf("Println() = %q, want %q", buf.String(), want)
	}
}

func TestWarn(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetErrOutput(&buf)

	Warn("something is wrong")

	want := "Warning: something is wrong\n"
	if buf.String() != want {
		t.Errorf("Warn() = %q, want %q", buf.String(), want)
	}
}

func TestWarn_WithFormat(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetErrOutput(&buf)

	Warn("failed to load %s: %d errors", "config", 3)

	want := "Warning: failed to load config: 3 errors\n"
	if buf.String() != want {
		t.Errorf("Warn() = %q, want %q", buf.String(), want)
	}
}

func TestError(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetErrOutput(&buf)

	Error("something failed")

	want := "Error: something failed\n"
	if buf.String() != want {
		t.Errorf("Error() = %q, want %q", buf.String(), want)
	}
}

func TestError_WithFormat(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetErrOutput(&buf)

	Error("failed with code %d", 42)

	want := "Error: failed with code 42\n"
	if buf.String() != want {
		t.Errorf("Error() = %q, want %q", buf.String(), want)
	}
}

func TestSilentMode(t *testing.T) {
	defer Reset()

	var stdoutBuf, stderrBuf bytes.Buffer
	SetOutput(&stdoutBuf)
	SetErrOutput(&stderrBuf)

	SetSilent(true)

	Print("print")
	Printf("printf")
	Println("println")
	Warn("warning")
	Error("error")

	// Print* should be suppressed
	if stdoutBuf.Len() > 0 {
		t.Errorf("Print* should be suppressed in silent mode, got: %q", stdoutBuf.String())
	}

	// Warn and Error should NOT be suppressed
	if !strings.Contains(stderrBuf.String(), "warning") {
		t.Errorf("Warn should not be suppressed in silent mode")
	}
	if !strings.Contains(stderrBuf.String(), "error") {
		t.Errorf("Error should not be suppressed in silent mode")
	}
}

func TestIsSilent(t *testing.T) {
	defer Reset()

	if IsSilent() {
		t.Errorf("IsSilent() should be false by default")
	}

	SetSilent(true)
	if !IsSilent() {
		t.Errorf("IsSilent() should be true after SetSilent(true)")
	}

	SetSilent(false)
	if IsSilent() {
		t.Errorf("IsSilent() should be false after SetSilent(false)")
	}
}

func TestStdout(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetOutput(&buf)

	w := Stdout()
	_, _ = w.Write([]byte("test"))

	if buf.String() != "test" {
		t.Errorf("Stdout() writer = %q, want %q", buf.String(), "test")
	}
}

func TestStdout_Silent(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetOutput(&buf)
	SetSilent(true)

	w := Stdout()
	_, _ = w.Write([]byte("test"))

	// Should be discarded
	if buf.Len() > 0 {
		t.Errorf("Stdout() should return discard writer in silent mode, got: %q", buf.String())
	}
}

func TestStderr(t *testing.T) {
	defer Reset()

	var buf bytes.Buffer
	SetErrOutput(&buf)

	w := Stderr()
	_, _ = w.Write([]byte("test"))

	if buf.String() != "test" {
		t.Errorf("Stderr() writer = %q, want %q", buf.String(), "test")
	}
}

func TestSetOutput_Nil(t *testing.T) {
	defer Reset()

	// Setting nil should reset to os.Stdout (not panic)
	SetOutput(nil)
	SetErrOutput(nil)

	// Should not panic
	Print("test")
	Warn("test")
}

func TestDiscard(t *testing.T) {
	defer Reset()
	Discard()

	// Should not panic
	Print("test")
	Printf("test %d", 1)
	Println("test")
	Warn("test")
	Error("test")
}

func TestReset(t *testing.T) {
	SetSilent(true)
	var buf bytes.Buffer
	SetOutput(&buf)

	Reset()

	if IsSilent() {
		t.Errorf("Reset() should clear silent mode")
	}
}
