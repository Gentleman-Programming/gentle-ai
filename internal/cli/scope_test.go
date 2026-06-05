package cli

import (
	"os"
	"testing"
)

func TestResolveInstallScope(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		envValue  string
		want      InstallScope
		wantErr   bool
	}{
		{
			name:      "empty flag and no env defaults to global",
			flagValue: "",
			envValue:  "",
			want:      ScopeGlobal,
		},
		{
			name:      "flag global returns global",
			flagValue: "global",
			envValue:  "",
			want:      ScopeGlobal,
		},
		{
			name:      "flag workspace returns workspace",
			flagValue: "workspace",
			envValue:  "",
			want:      ScopeWorkspace,
		},
		{
			name:      "env global returns global",
			flagValue: "",
			envValue:  "global",
			want:      ScopeGlobal,
		},
		{
			name:      "env workspace returns workspace",
			flagValue: "",
			envValue:  "workspace",
			want:      ScopeWorkspace,
		},
		{
			name:      "flag takes precedence over env",
			flagValue: "workspace",
			envValue:  "global",
			want:      ScopeWorkspace,
		},
		{
			name:      "invalid flag value returns error",
			flagValue: "project",
			wantErr:   true,
		},
		{
			name:     "invalid env value returns error",
			envValue: "local",
			wantErr:  true,
		},
		{
			name:      "whitespace-only flag treated as unset",
			flagValue: "   ",
			envValue:  "",
			want:      ScopeGlobal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(scopeEnvVar, tt.envValue)
			} else {
				os.Unsetenv(scopeEnvVar)
			}

			got, err := ResolveInstallScope(tt.flagValue)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveInstallScope(%q) error = %v, wantErr %v", tt.flagValue, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("ResolveInstallScope(%q) = %q, want %q", tt.flagValue, got, tt.want)
			}
		})
	}
}

func TestResolveAgentConfigDir(t *testing.T) {
	tests := []struct {
		name         string
		scope        InstallScope
		homeDir      string
		workspaceDir string
		want         string
	}{
		{
			name:         "global scope uses homeDir",
			scope:        ScopeGlobal,
			homeDir:      "/home/user",
			workspaceDir: "/projects/myapp",
			want:         "/home/user",
		},
		{
			name:         "workspace scope uses workspaceDir",
			scope:        ScopeWorkspace,
			homeDir:      "/home/user",
			workspaceDir: "/projects/myapp",
			want:         "/projects/myapp",
		},
		{
			name:         "workspace scope with empty workspaceDir falls back to homeDir",
			scope:        ScopeWorkspace,
			homeDir:      "/home/user",
			workspaceDir: "",
			want:         "/home/user",
		},
		{
			name:         "workspace scope with whitespace-only workspaceDir falls back to homeDir",
			scope:        ScopeWorkspace,
			homeDir:      "/home/user",
			workspaceDir: "   ",
			want:         "/home/user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAgentConfigDir(tt.scope, tt.homeDir, tt.workspaceDir)
			if got != tt.want {
				t.Fatalf("ResolveAgentConfigDir(%q, %q, %q) = %q, want %q",
					tt.scope, tt.homeDir, tt.workspaceDir, got, tt.want)
			}
		})
	}
}
