package shellinstaller

import "testing"

// The supervised program must carry the program identity, or gentle-pi never
// publishes its desktop feed and Gentle Shell Desktop shows no session
// (Gentleman-Programming/gentle-shell#3419).
func TestUserProgramIdentityEnv(t *testing.T) {
	env := userProgramIdentityEnv()
	if len(env) != 1 || env[0] != "PI_PROGRAM_STATUS_APP=gentle-shell" {
		t.Fatalf("userProgramIdentityEnv() = %q, want [PI_PROGRAM_STATUS_APP=gentle-shell]", env)
	}
}
