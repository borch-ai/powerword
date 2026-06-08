package loop

import (
	"bytes"
	"strings"
	"testing"
)

func TestGuard_Authorize_NonInteractive(t *testing.T) {
	tests := []struct {
		name         string
		profile      SecurityProfile
		toolName     string
		input        string
		wantResult   bool
		wantErr      bool
		errMatch     string
		expectPrompt bool
	}{
		{
			name:         "Bypass profile allows everything",
			profile:      Bypass,
			toolName:     "filesystem.delete_file",
			wantResult:   true,
			wantErr:      false,
			expectPrompt: false,
		},
		{
			name:         "ReadOnly profile blocks mutating tool",
			profile:      ReadOnly,
			toolName:     "filesystem.delete_file",
			wantResult:   false,
			wantErr:      true,
			errMatch:     "blocked by ReadOnly",
			expectPrompt: false,
		},
		{
			name:         "ReadOnly profile allows read tool",
			profile:      ReadOnly,
			toolName:     "filesystem.list_dir",
			wantResult:   true,
			wantErr:      false,
			expectPrompt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bytes.NewBufferString(tt.input)
			out := &bytes.Buffer{}
			g := NewGuard(tt.profile, in, out)

			gotResult, err := g.Authorize(tt.toolName, nil)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Authorize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
				t.Errorf("Authorize() error = %v, errMatch %v", err, tt.errMatch)
			}
			if gotResult != tt.wantResult {
				t.Errorf("Authorize() gotResult = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestGuard_Authorize_Interactive(t *testing.T) {
	tests := []struct {
		name         string
		profile      SecurityProfile
		toolName     string
		input        string
		wantResult   bool
		wantErr      bool
		errMatch     string
		expectPrompt bool
	}{
		{
			name:         "Interactive profile allows read tool without prompt",
			profile:      Interactive,
			toolName:     "filesystem.list_dir",
			input:        "", // EOF immediately
			wantResult:   true,
			wantErr:      false,
			expectPrompt: false,
		},
		{
			name:         "Interactive profile allows on yes without newline",
			profile:      Interactive,
			toolName:     "filesystem.delete_file",
			input:        "y", // no newline, EOF immediately
			wantResult:   true,
			wantErr:      false,
			expectPrompt: true,
		},
		{
			name:         "Interactive profile allows on yes",
			profile:      Interactive,
			toolName:     "filesystem.delete_file",
			input:        "y\n",
			wantResult:   true,
			wantErr:      false,
			expectPrompt: true,
		},
		{
			name:         "Interactive profile allows on Yes",
			profile:      Interactive,
			toolName:     "filesystem.delete_file",
			input:        "Yes\n",
			wantResult:   true,
			wantErr:      false,
			expectPrompt: true,
		},
		{
			name:         "Interactive profile denies on no",
			profile:      Interactive,
			toolName:     "filesystem.delete_file",
			input:        "n\n",
			wantResult:   false,
			wantErr:      false,
			expectPrompt: true,
		},
		{
			name:         "Interactive profile denies on empty",
			profile:      Interactive,
			toolName:     "filesystem.delete_file",
			input:        "\n",
			wantResult:   false,
			wantErr:      false,
			expectPrompt: true,
		},
		{
			name:         "Interactive profile fails closed on EOF",
			profile:      Interactive,
			toolName:     "filesystem.delete_file",
			input:        "", // EOF immediately
			wantResult:   false,
			wantErr:      false,
			expectPrompt: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := bytes.NewBufferString(tt.input)
			out := &bytes.Buffer{}
			g := NewGuard(tt.profile, in, out)

			gotResult, err := g.Authorize(tt.toolName, nil)

			if (err != nil) != tt.wantErr {
				t.Fatalf("Authorize() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
				t.Errorf("Authorize() error = %v, errMatch %v", err, tt.errMatch)
			}
			if gotResult != tt.wantResult {
				t.Errorf("Authorize() gotResult = %v, want %v", gotResult, tt.wantResult)
			}

			if tt.expectPrompt {
				outStr := out.String()
				if !strings.Contains(outStr, tt.toolName) {
					t.Errorf("expected output to contain tool name %q, got: %q", tt.toolName, outStr)
				}
			}
		})
	}
}
