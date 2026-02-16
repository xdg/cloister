package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStdinPrompter_SelectsOption(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		options    []string
		defaultIdx int
		wantIdx    int
		wantErr    bool
	}{
		{
			name:       "select first option",
			input:      "1\n",
			options:    []string{"Option A", "Option B", "Option C"},
			defaultIdx: 0,
			wantIdx:    0,
		},
		{
			name:       "select second option",
			input:      "2\n",
			options:    []string{"Option A", "Option B", "Option C"},
			defaultIdx: 0,
			wantIdx:    1,
		},
		{
			name:       "select third option",
			input:      "3\n",
			options:    []string{"Option A", "Option B", "Option C"},
			defaultIdx: 0,
			wantIdx:    2,
		},
		{
			name:       "empty input returns default",
			input:      "\n",
			options:    []string{"Option A", "Option B", "Option C"},
			defaultIdx: 0,
			wantIdx:    0,
		},
		{
			name:       "empty input with non-zero default",
			input:      "\n",
			options:    []string{"Option A", "Option B", "Option C"},
			defaultIdx: 1,
			wantIdx:    1,
		},
		{
			name:       "whitespace input returns default",
			input:      "   \n",
			options:    []string{"Option A", "Option B"},
			defaultIdx: 0,
			wantIdx:    0,
		},
		{
			name:       "invalid selection - not a number",
			input:      "abc\n",
			options:    []string{"Option A", "Option B"},
			defaultIdx: 0,
			wantErr:    true,
		},
		{
			name:       "invalid selection - zero",
			input:      "0\n",
			options:    []string{"Option A", "Option B"},
			defaultIdx: 0,
			wantErr:    true,
		},
		{
			name:       "invalid selection - too high",
			input:      "5\n",
			options:    []string{"Option A", "Option B"},
			defaultIdx: 0,
			wantErr:    true,
		},
		{
			name:       "invalid selection - negative",
			input:      "-1\n",
			options:    []string{"Option A", "Option B"},
			defaultIdx: 0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdinPrompter(in, out)

			got, err := p.Prompt("Select:", tt.options, tt.defaultIdx)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if got != tt.wantIdx {
				t.Errorf("got index %d, want %d", got, tt.wantIdx)
			}
		})
	}
}

func TestStdinPrompter_DisplaysOptions(t *testing.T) {
	in := strings.NewReader("1\n")
	out := &bytes.Buffer{}
	p := NewStdinPrompter(in, out)

	options := []string{
		"Use existing Claude login (recommended)",
		"Long-lived OAuth token",
		"API key",
	}

	_, err := p.Prompt("Select authentication method:", options, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()

	// Check prompt is displayed
	if !strings.Contains(output, "Select authentication method:") {
		t.Error("output should contain prompt")
	}

	// Check all options are displayed
	for i, opt := range options {
		expectedNum := i + 1 // 1-indexed for display
		if !strings.Contains(output, opt) {
			t.Errorf("output should contain option %q", opt)
		}
		if !strings.Contains(output, fmt.Sprintf("%d.", expectedNum)) {
			t.Errorf("output should contain number %d.", expectedNum)
		}
	}

	// Check default is marked
	if !strings.Contains(output, "(default)") {
		t.Error("output should mark default option")
	}
}

func TestStdinPrompter_ValidationErrors(t *testing.T) {
	in := strings.NewReader("1\n")
	out := &bytes.Buffer{}
	p := NewStdinPrompter(in, out)

	// No options
	_, err := p.Prompt("Select:", []string{}, 0)
	if err == nil {
		t.Error("expected error for empty options")
	}

	// Default out of range (negative)
	_, err = p.Prompt("Select:", []string{"A", "B"}, -1)
	if err == nil {
		t.Error("expected error for negative default index")
	}

	// Default out of range (too high)
	_, err = p.Prompt("Select:", []string{"A", "B"}, 2)
	if err == nil {
		t.Error("expected error for default index >= len(options)")
	}
}

func TestMockPrompter_ReturnsConfiguredResponses(t *testing.T) {
	m := NewMockPrompter(1, 2, 0)

	// First call returns 1
	got, err := m.Prompt("Prompt 1", []string{"A", "B", "C"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}

	// Second call returns 2
	got, err = m.Prompt("Prompt 2", []string{"A", "B", "C"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}

	// Third call returns 0
	got, err = m.Prompt("Prompt 3", []string{"A", "B", "C"}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestMockPrompter_RecordsCalls(t *testing.T) {
	m := NewMockPrompter(0)

	options := []string{"Option A", "Option B"}
	_, err := m.Prompt("Select:", options, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}

	call := m.Calls[0]
	if call.Prompt != "Select:" {
		t.Errorf("call.Prompt = %q, want %q", call.Prompt, "Select:")
	}
	if len(call.Options) != 2 {
		t.Errorf("call.Options has %d elements, want 2", len(call.Options))
	}
	if call.DefaultIdx != 1 {
		t.Errorf("call.DefaultIdx = %d, want 1", call.DefaultIdx)
	}
}

func TestMockPrompter_ReturnsErrors(t *testing.T) {
	testErr := errors.New("test error")
	m := &MockPrompter{
		Responses: []int{0},
		Errors:    []error{testErr},
	}

	_, err := m.Prompt("Select:", []string{"A"}, 0)
	if !errors.Is(err, testErr) {
		t.Errorf("got error %v, want %v", err, testErr)
	}
}

func TestMockPrompter_ReturnsDefaultWhenExhausted(t *testing.T) {
	m := NewMockPrompter() // No responses configured

	got, err := m.Prompt("Select:", []string{"A", "B"}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("got %d, want default 1", got)
	}
}

func TestAuthMethodPrompt_Integration(t *testing.T) {
	// Test the exact auth method prompt scenario from the spec
	in := strings.NewReader("\n") // User presses Enter (default)
	out := &bytes.Buffer{}
	p := NewStdinPrompter(in, out)

	options := []string{
		"Use existing Claude login (recommended)",
		"Long-lived OAuth token (from `claude setup-token`)",
		"API key (from console.anthropic.com)",
	}

	got, err := p.Prompt("Select authentication method:", options, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default is option 1 (index 0)
	if got != 0 {
		t.Errorf("empty input should select default (0), got %d", got)
	}

	// Verify output format
	output := out.String()
	if !strings.Contains(output, "1. Use existing Claude login (recommended) (default)") {
		t.Error("first option should be marked as default")
	}
	if !strings.Contains(output, "2. Long-lived OAuth token") {
		t.Error("second option should be present")
	}
	if !strings.Contains(output, "3. API key") {
		t.Error("third option should be present")
	}
}

// CredentialReader tests

func TestMockCredentialReader_ReturnsConfiguredCredentials(t *testing.T) {
	m := NewMockCredentialReader("token123", "apikey456")

	// First call returns "token123"
	got, err := m.ReadCredential("Enter token: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "token123" {
		t.Errorf("got %q, want %q", got, "token123")
	}

	// Second call returns "apikey456"
	got, err = m.ReadCredential("Enter API key: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "apikey456" {
		t.Errorf("got %q, want %q", got, "apikey456")
	}
}

func TestMockCredentialReader_RecordsCalls(t *testing.T) {
	m := NewMockCredentialReader("test-credential")

	_, err := m.ReadCredential("Paste your OAuth token: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}

	if m.Calls[0] != "Paste your OAuth token: " {
		t.Errorf("call prompt = %q, want %q", m.Calls[0], "Paste your OAuth token: ")
	}
}

func TestMockCredentialReader_ReturnsErrors(t *testing.T) {
	testErr := errors.New("test error")
	m := &MockCredentialReader{
		Credentials: []string{"cred"},
		Errors:      []error{testErr},
	}

	_, err := m.ReadCredential("Enter: ")
	if !errors.Is(err, testErr) {
		t.Errorf("got error %v, want %v", err, testErr)
	}
}

func TestMockCredentialReader_ReturnsEmptyWhenExhausted(t *testing.T) {
	m := NewMockCredentialReader() // No credentials configured

	got, err := m.ReadCredential("Enter: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestMockCredentialReader_OAuthTokenPrompt(t *testing.T) {
	// Test the exact OAuth token prompt from the spec
	m := NewMockCredentialReader("oauth-token-from-setup-token")

	got, err := m.ReadCredential("Paste your OAuth token (from `claude setup-token`): ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "oauth-token-from-setup-token" {
		t.Errorf("got %q, want %q", got, "oauth-token-from-setup-token")
	}

	// Verify the correct prompt was used
	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}
	expectedPrompt := "Paste your OAuth token (from `claude setup-token`): "
	if m.Calls[0] != expectedPrompt {
		t.Errorf("prompt = %q, want %q", m.Calls[0], expectedPrompt)
	}
}

func TestMockCredentialReader_APIKeyPrompt(t *testing.T) {
	// Test the exact API key prompt from the spec
	m := NewMockCredentialReader("sk-ant-api-key-here")

	got, err := m.ReadCredential("Paste your API key (from console.anthropic.com): ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sk-ant-api-key-here" {
		t.Errorf("got %q, want %q", got, "sk-ant-api-key-here")
	}

	// Verify the correct prompt was used
	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}
	expectedPrompt := "Paste your API key (from console.anthropic.com): "
	if m.Calls[0] != expectedPrompt {
		t.Errorf("prompt = %q, want %q", m.Calls[0], expectedPrompt)
	}
}

// YesNoPrompter tests

func TestStdinYesNoPrompter_DefaultYes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    bool
		wantErr bool
	}{
		{
			name:  "empty input returns true (default yes)",
			input: "\n",
			want:  true,
		},
		{
			name:  "whitespace input returns true (default yes)",
			input: "   \n",
			want:  true,
		},
		{
			name:  "y returns true",
			input: "y\n",
			want:  true,
		},
		{
			name:  "Y returns true",
			input: "Y\n",
			want:  true,
		},
		{
			name:  "yes returns true",
			input: "yes\n",
			want:  true,
		},
		{
			name:  "YES returns true",
			input: "YES\n",
			want:  true,
		},
		{
			name:  "n returns false",
			input: "n\n",
			want:  false,
		},
		{
			name:  "N returns false",
			input: "N\n",
			want:  false,
		},
		{
			name:  "no returns false",
			input: "no\n",
			want:  false,
		},
		{
			name:  "NO returns false",
			input: "NO\n",
			want:  false,
		},
		{
			name:    "invalid input returns error",
			input:   "maybe\n",
			wantErr: true,
		},
		{
			name:    "numeric input returns error",
			input:   "1\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdinYesNoPrompter(in, out)

			got, err := p.PromptYesNo("Continue? [Y/n]: ", true)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStdinYesNoPrompter_DefaultNo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "empty input returns false (default no)",
			input: "\n",
			want:  false,
		},
		{
			name:  "whitespace input returns false (default no)",
			input: "   \n",
			want:  false,
		},
		{
			name:  "y returns true (overrides default)",
			input: "y\n",
			want:  true,
		},
		{
			name:  "n returns false",
			input: "n\n",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdinYesNoPrompter(in, out)

			got, err := p.PromptYesNo("Continue? [y/N]: ", false)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStdinYesNoPrompter_DisplaysPrompt(t *testing.T) {
	in := strings.NewReader("y\n")
	out := &bytes.Buffer{}
	p := NewStdinYesNoPrompter(in, out)

	_, err := p.PromptYesNo("Skip Claude's built-in permission prompts? (recommended inside cloister) [Y/n]: ", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Skip Claude's built-in permission prompts?") {
		t.Error("output should contain the prompt text")
	}
}

func TestMockYesNoPrompter_ReturnsConfiguredResponses(t *testing.T) {
	m := NewMockYesNoPrompter(true, false, true)

	// First call returns true
	got, err := m.PromptYesNo("Prompt 1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("got %v, want true", got)
	}

	// Second call returns false
	got, err = m.PromptYesNo("Prompt 2", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Errorf("got %v, want false", got)
	}

	// Third call returns true
	got, err = m.PromptYesNo("Prompt 3", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("got %v, want true", got)
	}
}

func TestMockYesNoPrompter_RecordsCalls(t *testing.T) {
	m := NewMockYesNoPrompter(true)

	_, err := m.PromptYesNo("Continue? [Y/n]: ", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.Calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.Calls))
	}

	call := m.Calls[0]
	if call.Prompt != "Continue? [Y/n]: " {
		t.Errorf("call.Prompt = %q, want %q", call.Prompt, "Continue? [Y/n]: ")
	}
	if !call.DefaultYes {
		t.Errorf("call.DefaultYes = %v, want true", call.DefaultYes)
	}
}

func TestMockYesNoPrompter_ReturnsErrors(t *testing.T) {
	testErr := errors.New("test error")
	m := &MockYesNoPrompter{
		Responses: []bool{true},
		Errors:    []error{testErr},
	}

	_, err := m.PromptYesNo("Continue?", true)
	if !errors.Is(err, testErr) {
		t.Errorf("got error %v, want %v", err, testErr)
	}
}

func TestMockYesNoPrompter_ReturnsDefaultWhenExhausted(t *testing.T) {
	m := NewMockYesNoPrompter() // No responses configured

	// Should return the default (true)
	got, err := m.PromptYesNo("Continue?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("got %v, want default true", got)
	}

	// Should return the default (false)
	got, err = m.PromptYesNo("Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Errorf("got %v, want default false", got)
	}
}

func TestSkipPermissionsPrompt_Spec(t *testing.T) {
	// Test the exact skip-permissions prompt from the spec (Phase 3.2.5)
	// Prompt: "Skip Claude's built-in permission prompts? (recommended inside cloister) [Y/n]:"
	// Default to yes (Y) if user just presses Enter
	// Test: Empty input -> true; "n" -> false; "N" -> false; "y" -> true

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty input returns true", "\n", true},
		{"n returns false", "n\n", false},
		{"N returns false", "N\n", false},
		{"y returns true", "y\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			out := &bytes.Buffer{}
			p := NewStdinYesNoPrompter(in, out)

			got, err := p.PromptYesNo("Skip Claude's built-in permission prompts? (recommended inside cloister) [Y/n]: ", true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("input %q: got %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
