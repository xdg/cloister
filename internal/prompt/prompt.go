// Package prompt provides interfaces and implementations for interactive
// user prompts, designed for testability with mock implementations.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Prompter defines the interface for presenting options to a user and
// getting their selection.
type Prompter interface {
	// Prompt displays a prompt with numbered options and returns the
	// zero-based index of the selected option. If the user presses Enter
	// without input, defaultIdx is returned. Returns an error if the
	// selection is invalid or input cannot be read.
	Prompt(prompt string, options []string, defaultIdx int) (int, error)
}

// StdinPrompter implements Prompter using stdin/stdout for real terminal use.
type StdinPrompter struct {
	In  io.Reader
	Out io.Writer
}

// NewStdinPrompter creates a StdinPrompter that reads from r and writes to w.
func NewStdinPrompter(r io.Reader, w io.Writer) *StdinPrompter {
	return &StdinPrompter{In: r, Out: w}
}

// Prompt displays the prompt and options, then reads user input.
// Options are displayed as a numbered list (1-indexed for user display).
// The default option is marked with "(default)".
// Returns the zero-based index of the selected option.
func (p *StdinPrompter) Prompt(prompt string, options []string, defaultIdx int) (int, error) {
	if len(options) == 0 {
		return 0, fmt.Errorf("no options provided")
	}
	if defaultIdx < 0 || defaultIdx >= len(options) {
		return 0, fmt.Errorf("default index %d out of range [0, %d)", defaultIdx, len(options))
	}

	// Display the prompt
	_, _ = fmt.Fprintln(p.Out, prompt)

	// Display numbered options (1-indexed for user)
	for i, opt := range options {
		suffix := ""
		if i == defaultIdx {
			suffix = " (default)"
		}
		_, _ = fmt.Fprintf(p.Out, "  %d. %s%s\n", i+1, opt, suffix)
	}

	// Prompt for input
	_, _ = fmt.Fprintf(p.Out, "Enter selection [%d]: ", defaultIdx+1)

	// Read user input
	reader := bufio.NewReader(p.In)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("failed to read input: %w", err)
	}

	// Trim whitespace
	input := strings.TrimSpace(line)

	// Empty input means use default
	if input == "" {
		return defaultIdx, nil
	}

	// Parse the selection as a number (1-indexed)
	selection, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("invalid selection %q: must be a number", input)
	}

	// Convert from 1-indexed to 0-indexed
	idx := selection - 1
	if idx < 0 || idx >= len(options) {
		return 0, fmt.Errorf("selection %d out of range (1-%d)", selection, len(options))
	}

	return idx, nil
}

// MockPrompter implements Prompter for testing, returning pre-configured responses.
type MockPrompter struct {
	// Responses is a queue of responses to return for successive calls.
	// Each response is the zero-based index to return.
	Responses []int
	// Errors is a queue of errors to return for successive calls.
	// If non-nil, the error is returned instead of the response.
	Errors []error
	// Calls records all calls made to Prompt for verification.
	Calls []MockPrompterCall

	callIndex int
}

// MockPrompterCall records a single call to Prompt.
type MockPrompterCall struct {
	Prompt     string
	Options    []string
	DefaultIdx int
}

// NewMockPrompter creates a MockPrompter with the given responses.
func NewMockPrompter(responses ...int) *MockPrompter {
	return &MockPrompter{Responses: responses}
}

// Prompt returns the next pre-configured response or error.
func (m *MockPrompter) Prompt(prompt string, options []string, defaultIdx int) (int, error) {
	// Record the call
	m.Calls = append(m.Calls, MockPrompterCall{
		Prompt:     prompt,
		Options:    options,
		DefaultIdx: defaultIdx,
	})

	// Check for error
	if m.callIndex < len(m.Errors) && m.Errors[m.callIndex] != nil {
		err := m.Errors[m.callIndex]
		m.callIndex++
		return 0, err
	}

	// Return configured response
	if m.callIndex < len(m.Responses) {
		response := m.Responses[m.callIndex]
		m.callIndex++
		return response, nil
	}

	// No more responses configured, return default
	m.callIndex++
	return defaultIdx, nil
}

// CredentialReader defines the interface for reading sensitive credentials
// from the user with hidden input.
type CredentialReader interface {
	// ReadCredential displays a prompt and reads a credential with hidden input.
	// The input is not echoed to the terminal for security.
	// Returns the credential string or an error if input cannot be read.
	ReadCredential(prompt string) (string, error)
}

// TerminalCredentialReader implements CredentialReader using golang.org/x/term
// for secure hidden input from a real terminal.
type TerminalCredentialReader struct {
	In  *os.File
	Out io.Writer
}

// NewTerminalCredentialReader creates a TerminalCredentialReader that reads
// from the given file (typically os.Stdin) and writes prompts to w.
func NewTerminalCredentialReader(in *os.File, out io.Writer) *TerminalCredentialReader {
	return &TerminalCredentialReader{In: in, Out: out}
}

// ReadCredential displays the prompt and reads input with echoing disabled.
func (r *TerminalCredentialReader) ReadCredential(prompt string) (string, error) {
	_, _ = fmt.Fprint(r.Out, prompt)

	// Read password with terminal echo disabled
	credential, err := term.ReadPassword(int(r.In.Fd()))
	if err != nil {
		return "", fmt.Errorf("failed to read credential: %w", err)
	}

	// Print newline since ReadPassword doesn't echo it
	_, _ = fmt.Fprintln(r.Out)

	return string(credential), nil
}

// MockCredentialReader implements CredentialReader for testing,
// returning pre-configured credentials.
type MockCredentialReader struct {
	// Credentials is a queue of credentials to return for successive calls.
	Credentials []string
	// Errors is a queue of errors to return for successive calls.
	// If non-nil, the error is returned instead of the credential.
	Errors []error
	// Calls records all prompts passed to ReadCredential for verification.
	Calls []string

	callIndex int
}

// NewMockCredentialReader creates a MockCredentialReader with the given credentials.
func NewMockCredentialReader(credentials ...string) *MockCredentialReader {
	return &MockCredentialReader{Credentials: credentials}
}

// ReadCredential returns the next pre-configured credential or error.
func (m *MockCredentialReader) ReadCredential(prompt string) (string, error) {
	// Record the call
	m.Calls = append(m.Calls, prompt)

	// Check for error
	if m.callIndex < len(m.Errors) && m.Errors[m.callIndex] != nil {
		err := m.Errors[m.callIndex]
		m.callIndex++
		return "", err
	}

	// Return configured credential
	if m.callIndex < len(m.Credentials) {
		credential := m.Credentials[m.callIndex]
		m.callIndex++
		return credential, nil
	}

	// No more credentials configured, return empty string
	m.callIndex++
	return "", nil
}

// YesNoPrompter defines the interface for yes/no confirmation prompts.
type YesNoPrompter interface {
	// PromptYesNo displays a yes/no prompt and returns the user's response.
	// If the user presses Enter without input, defaultYes determines the result.
	// Returns true for yes, false for no.
	PromptYesNo(prompt string, defaultYes bool) (bool, error)
}

// StdinYesNoPrompter implements YesNoPrompter using stdin/stdout.
type StdinYesNoPrompter struct {
	In  io.Reader
	Out io.Writer
}

// NewStdinYesNoPrompter creates a StdinYesNoPrompter that reads from r and writes to w.
func NewStdinYesNoPrompter(r io.Reader, w io.Writer) *StdinYesNoPrompter {
	return &StdinYesNoPrompter{In: r, Out: w}
}

// PromptYesNo displays the prompt and reads user input.
// Accepts "y", "Y", "yes", "YES" as true; "n", "N", "no", "NO" as false.
// Empty input returns defaultYes.
func (p *StdinYesNoPrompter) PromptYesNo(prompt string, defaultYes bool) (bool, error) {
	_, _ = fmt.Fprint(p.Out, prompt)

	reader := bufio.NewReader(p.In)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	input := strings.TrimSpace(strings.ToLower(line))

	// Empty input means use default
	if input == "" {
		return defaultYes, nil
	}

	// Check for yes
	if input == "y" || input == "yes" {
		return true, nil
	}

	// Check for no
	if input == "n" || input == "no" {
		return false, nil
	}

	return false, fmt.Errorf("invalid input %q: expected y/n", input)
}

// MockYesNoPrompter implements YesNoPrompter for testing.
type MockYesNoPrompter struct {
	// Responses is a queue of responses to return for successive calls.
	Responses []bool
	// Errors is a queue of errors to return for successive calls.
	Errors []error
	// Calls records all calls made to PromptYesNo for verification.
	Calls []MockYesNoCall

	callIndex int
}

// MockYesNoCall records a single call to PromptYesNo.
type MockYesNoCall struct {
	Prompt     string
	DefaultYes bool
}

// NewMockYesNoPrompter creates a MockYesNoPrompter with the given responses.
func NewMockYesNoPrompter(responses ...bool) *MockYesNoPrompter {
	return &MockYesNoPrompter{Responses: responses}
}

// PromptYesNo returns the next pre-configured response or error.
func (m *MockYesNoPrompter) PromptYesNo(prompt string, defaultYes bool) (bool, error) {
	// Record the call
	m.Calls = append(m.Calls, MockYesNoCall{
		Prompt:     prompt,
		DefaultYes: defaultYes,
	})

	// Check for error
	if m.callIndex < len(m.Errors) && m.Errors[m.callIndex] != nil {
		err := m.Errors[m.callIndex]
		m.callIndex++
		return false, err
	}

	// Return configured response
	if m.callIndex < len(m.Responses) {
		response := m.Responses[m.callIndex]
		m.callIndex++
		return response, nil
	}

	// No more responses configured, return default
	m.callIndex++
	return defaultYes, nil
}
