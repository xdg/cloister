package request

import (
	"encoding/json"
	"testing"
)

func TestCommandRequestRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		request CommandRequest
	}{
		{
			name:    "simple_args",
			request: CommandRequest{Args: []string{"docker", "compose", "ps"}},
		},
		{
			name:    "args_with_flags",
			request: CommandRequest{Args: []string{"docker", "compose", "up", "-d", "--build"}},
		},
		{
			name:    "deprecated_cmd_still_works",
			request: CommandRequest{Cmd: "legacy", Args: []string{"echo", "hello"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(&tc.request)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var roundTripped CommandRequest
			if err := json.Unmarshal(data, &roundTripped); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if roundTripped.Cmd != tc.request.Cmd {
				t.Errorf("Cmd = %q, want %q", roundTripped.Cmd, tc.request.Cmd)
			}
			if len(roundTripped.Args) != len(tc.request.Args) {
				t.Errorf("Args length = %d, want %d", len(roundTripped.Args), len(tc.request.Args))
			}
			for i := range tc.request.Args {
				if roundTripped.Args[i] != tc.request.Args[i] {
					t.Errorf("Args[%d] = %q, want %q", i, roundTripped.Args[i], tc.request.Args[i])
				}
			}
		})
	}
}

func TestCommandResponseRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		response CommandResponse
	}{
		{
			name: "approved",
			response: CommandResponse{
				Status:   "approved",
				ExitCode: 0,
				Stdout:   "container1 running\ncontainer2 running\n",
				Stderr:   "",
			},
		},
		{
			name: "approved_with_nonzero_exit",
			response: CommandResponse{
				Status:   "approved",
				ExitCode: 1,
				Stdout:   "",
				Stderr:   "Error: container not found",
			},
		},
		{
			name: "auto_approved",
			response: CommandResponse{
				Status:   "auto_approved",
				Pattern:  "^docker compose ps$",
				ExitCode: 0,
				Stdout:   "NAME   STATUS\n",
				Stderr:   "",
			},
		},
		{
			name: "denied",
			response: CommandResponse{
				Status: "denied",
				Reason: "Command not in approved list",
			},
		},
		{
			name: "denied_by_user",
			response: CommandResponse{
				Status: "denied",
				Reason: "Denied by user",
			},
		},
		{
			name: "timeout",
			response: CommandResponse{
				Status: "timeout",
				Reason: "Request timed out after 5m",
			},
		},
		{
			name: "error",
			response: CommandResponse{
				Status: "error",
				Reason: "Internal server error: connection refused",
			},
		},
		{
			name: "error_command_not_found",
			response: CommandResponse{
				Status: "error",
				Reason: "executable not found: nonexistent",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(&tc.response)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var roundTripped CommandResponse
			if err := json.Unmarshal(data, &roundTripped); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if roundTripped.Status != tc.response.Status {
				t.Errorf("Status = %q, want %q", roundTripped.Status, tc.response.Status)
			}
			if roundTripped.Pattern != tc.response.Pattern {
				t.Errorf("Pattern = %q, want %q", roundTripped.Pattern, tc.response.Pattern)
			}
			if roundTripped.Reason != tc.response.Reason {
				t.Errorf("Reason = %q, want %q", roundTripped.Reason, tc.response.Reason)
			}
			if roundTripped.ExitCode != tc.response.ExitCode {
				t.Errorf("ExitCode = %d, want %d", roundTripped.ExitCode, tc.response.ExitCode)
			}
			if roundTripped.Stdout != tc.response.Stdout {
				t.Errorf("Stdout = %q, want %q", roundTripped.Stdout, tc.response.Stdout)
			}
			if roundTripped.Stderr != tc.response.Stderr {
				t.Errorf("Stderr = %q, want %q", roundTripped.Stderr, tc.response.Stderr)
			}
		})
	}
}

func TestCommandResponseOmitEmpty(t *testing.T) {
	tests := []struct {
		name           string
		response       CommandResponse
		expectedFields []string
		omittedFields  []string
	}{
		{
			name: "approved_omits_pattern_and_reason",
			response: CommandResponse{
				Status:   "approved",
				ExitCode: 1,
				Stdout:   "output",
			},
			expectedFields: []string{"status", "exit_code", "stdout"},
			omittedFields:  []string{"pattern", "reason"},
		},
		{
			name: "auto_approved_includes_pattern",
			response: CommandResponse{
				Status:   "auto_approved",
				Pattern:  "^docker compose ps$",
				ExitCode: 1,
			},
			expectedFields: []string{"status", "pattern", "exit_code"},
			omittedFields:  []string{"reason"},
		},
		{
			name: "denied_omits_stdout_stderr",
			response: CommandResponse{
				Status: "denied",
				Reason: "Not allowed",
			},
			expectedFields: []string{"status", "reason", "exit_code"},
			omittedFields:  []string{"pattern", "stdout", "stderr"},
		},
		{
			name: "timeout_omits_stdout_stderr",
			response: CommandResponse{
				Status: "timeout",
				Reason: "Request timed out",
			},
			expectedFields: []string{"status", "reason", "exit_code"},
			omittedFields:  []string{"pattern", "stdout", "stderr"},
		},
		{
			name: "error_omits_stdout_stderr",
			response: CommandResponse{
				Status: "error",
				Reason: "Internal error",
			},
			expectedFields: []string{"status", "reason", "exit_code"},
			omittedFields:  []string{"pattern", "stdout", "stderr"},
		},
		{
			name: "empty_strings_omitted",
			response: CommandResponse{
				Status:   "approved",
				ExitCode: 1,
				Stdout:   "",
				Stderr:   "",
			},
			expectedFields: []string{"status", "exit_code"},
			omittedFields:  []string{"pattern", "reason", "stdout", "stderr"},
		},
		{
			name: "zero_exit_code_present",
			response: CommandResponse{
				Status:   "approved",
				ExitCode: 0,
			},
			expectedFields: []string{"status", "exit_code"},
			omittedFields:  []string{"pattern", "reason", "stdout", "stderr"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(&tc.response)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Unmarshal into a map to check field presence
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("failed to unmarshal to map: %v", err)
			}

			// Verify expected fields are present
			for _, field := range tc.expectedFields {
				if _, ok := result[field]; !ok {
					t.Errorf("expected field %q to be present, but it was omitted", field)
				}
			}

			// Verify omitted fields are absent
			for _, field := range tc.omittedFields {
				if _, ok := result[field]; ok {
					t.Errorf("expected field %q to be omitted, but it was present with value %v", field, result[field])
				}
			}
		})
	}
}

func TestCommandResponseJSONFieldNames(t *testing.T) {
	response := CommandResponse{
		Status:   "auto_approved",
		Pattern:  "^test$",
		Reason:   "test reason",
		ExitCode: 42,
		Stdout:   "stdout",
		Stderr:   "stderr",
	}

	data, err := json.Marshal(&response)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Verify snake_case field names
	expectedKeys := map[string]bool{
		"status":    true,
		"pattern":   true,
		"reason":    true,
		"exit_code": true,
		"stdout":    true,
		"stderr":    true,
	}

	for key := range result {
		if !expectedKeys[key] {
			t.Errorf("unexpected JSON field name: %q", key)
		}
	}

	// Verify all expected keys are present
	for key := range expectedKeys {
		if _, ok := result[key]; !ok {
			t.Errorf("expected JSON field %q not found", key)
		}
	}

	// Verify camelCase is NOT used
	unexpectedKeys := []string{
		"exitCode",
		"ExitCode",
	}
	for _, key := range unexpectedKeys {
		if _, ok := result[key]; ok {
			t.Errorf("JSON output should not contain camelCase field %q", key)
		}
	}
}

func TestCommandRequestJSONFieldNames(t *testing.T) {
	// Test that args is always present (required field)
	request := CommandRequest{
		Args: []string{"test", "command"},
	}

	data, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Verify "args" field is present
	if _, ok := result["args"]; !ok {
		t.Error("expected JSON field \"args\" not found")
	}

	// Verify "cmd" is omitted when empty (omitempty)
	if _, ok := result["cmd"]; ok {
		t.Error("expected \"cmd\" to be omitted when empty")
	}

	// Verify only args field is present
	if len(result) != 1 {
		t.Errorf("expected 1 JSON field, got %d: %v", len(result), result)
	}
}

func TestCommandRequestCmdOmitEmpty(t *testing.T) {
	// Test that cmd is omitted when empty
	request := CommandRequest{
		Args: []string{"echo", "hello"},
	}

	data, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, ok := result["cmd"]; ok {
		t.Error("cmd should be omitted when empty")
	}

	// Test that cmd is present when set (for backwards compat)
	request2 := CommandRequest{
		Cmd:  "legacy command",
		Args: []string{"echo", "hello"},
	}

	data2, err := json.Marshal(&request2)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result2 map[string]interface{}
	if err := json.Unmarshal(data2, &result2); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	if _, ok := result2["cmd"]; !ok {
		t.Error("cmd should be present when non-empty")
	}
}
