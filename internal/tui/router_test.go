package tui

import (
	"testing"
)

func TestNextScreen(t *testing.T) {
	tests := []struct {
		name       string
		fromScreen Screen
		wantScreen Screen
		wantOk     bool
	}{
		{
			name:       "Welcome to Detection (valid forward)",
			fromScreen: ScreenWelcome,
			wantScreen: ScreenDetection,
			wantOk:     true,
		},
		{
			name:       "Detection to Agents (valid forward)",
			fromScreen: ScreenDetection,
			wantScreen: ScreenAgents,
			wantOk:     true,
		},
		{
			name:       "Complete (no forward route, default unknown)",
			fromScreen: ScreenComplete,
			wantScreen: ScreenUnknown,
			wantOk:     false,
		},
		{
			name:       "UninstallResult (no forward route, not in map)",
			fromScreen: ScreenUninstallResult,
			wantScreen: ScreenUnknown,
			wantOk:     false,
		},
		{
			name:       "Unknown Screen",
			fromScreen: ScreenUnknown,
			wantScreen: ScreenUnknown,
			wantOk:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScreen, gotOk := NextScreen(tt.fromScreen)
			if gotScreen != tt.wantScreen {
				t.Errorf("NextScreen(%v) gotScreen = %v, want %v", tt.fromScreen, gotScreen, tt.wantScreen)
			}
			if gotOk != tt.wantOk {
				t.Errorf("NextScreen(%v) gotOk = %v, want %v", tt.fromScreen, gotOk, tt.wantOk)
			}
		})
	}
}

func TestPreviousScreen(t *testing.T) {
	tests := []struct {
		name       string
		fromScreen Screen
		wantScreen Screen
		wantOk     bool
	}{
		{
			name:       "Detection to Welcome (valid backward)",
			fromScreen: ScreenDetection,
			wantScreen: ScreenWelcome,
			wantOk:     true,
		},
		{
			name:       "Welcome (no backward route)",
			fromScreen: ScreenWelcome,
			wantScreen: ScreenUnknown,
			wantOk:     false,
		},
		{
			name:       "UninstallMode to Welcome (valid backward)",
			fromScreen: ScreenUninstallMode,
			wantScreen: ScreenWelcome,
			wantOk:     true,
		},
		{
			name:       "Unknown Screen",
			fromScreen: ScreenUnknown,
			wantScreen: ScreenUnknown,
			wantOk:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScreen, gotOk := PreviousScreen(tt.fromScreen)
			if gotScreen != tt.wantScreen {
				t.Errorf("PreviousScreen(%v) gotScreen = %v, want %v", tt.fromScreen, gotScreen, tt.wantScreen)
			}
			if gotOk != tt.wantOk {
				t.Errorf("PreviousScreen(%v) gotOk = %v, want %v", tt.fromScreen, gotOk, tt.wantOk)
			}
		})
	}
}
