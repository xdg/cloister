package cmd

import "testing"

func TestParseCloisterName(t *testing.T) {
	tests := []struct {
		name         string
		cloisterName string
		wantProject  string
		wantBranch   string
	}{
		{
			name:         "simple project and branch",
			cloisterName: "myproject-main",
			wantProject:  "myproject",
			wantBranch:   "main",
		},
		{
			name:         "branch with hyphen",
			cloisterName: "myproject-feature-foo",
			wantProject:  "myproject-feature",
			wantBranch:   "foo",
		},
		{
			name:         "project only (no branch hyphen)",
			cloisterName: "myproject",
			wantProject:  "myproject",
			wantBranch:   "",
		},
		{
			name:         "real world example",
			cloisterName: "cloister-main",
			wantProject:  "cloister",
			wantBranch:   "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProject, gotBranch := parseCloisterName(tt.cloisterName)
			if gotProject != tt.wantProject {
				t.Errorf("parseCloisterName(%q) project = %q, want %q", tt.cloisterName, gotProject, tt.wantProject)
			}
			if gotBranch != tt.wantBranch {
				t.Errorf("parseCloisterName(%q) branch = %q, want %q", tt.cloisterName, gotBranch, tt.wantBranch)
			}
		})
	}
}
