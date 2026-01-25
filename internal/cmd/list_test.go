package cmd

import "testing"

func TestParseContainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		wantProject   string
		wantBranch    string
	}{
		{
			name:          "simple project and branch",
			containerName: "cloister-myproject-main",
			wantProject:   "myproject",
			wantBranch:    "main",
		},
		{
			name:          "branch with hyphen",
			containerName: "cloister-myproject-feature-foo",
			wantProject:   "myproject-feature",
			wantBranch:    "foo",
		},
		{
			name:          "project only (no branch hyphen)",
			containerName: "cloister-myproject",
			wantProject:   "myproject",
			wantBranch:    "",
		},
		{
			name:          "no cloister prefix",
			containerName: "other-container",
			wantProject:   "other-container",
			wantBranch:    "",
		},
		{
			name:          "real world example",
			containerName: "cloister-cloister-main",
			wantProject:   "cloister",
			wantBranch:    "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProject, gotBranch := parseContainerName(tt.containerName)
			if gotProject != tt.wantProject {
				t.Errorf("parseContainerName(%q) project = %q, want %q", tt.containerName, gotProject, tt.wantProject)
			}
			if gotBranch != tt.wantBranch {
				t.Errorf("parseContainerName(%q) branch = %q, want %q", tt.containerName, gotBranch, tt.wantBranch)
			}
		})
	}
}
