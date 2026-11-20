//go:build linux || darwin

package shellinstaller

import "testing"

func TestUserDescriptorCeilingClampsToKernelAndInheritedMaximum(t *testing.T) {
	for _, tc := range []struct {
		name               string
		inherited, kernel  uint64
		wantSoft, wantHard uint64
	}{
		{"unlimited inherited, low kernel", ^uint64(0), 245760, 1024, 245760},
		{"unlimited inherited, high kernel", ^uint64(0), 1048576, 1024, 524288},
		{"inherited below kernel", 10240, 245760, 1024, 10240},
		{"kernel below soft default", ^uint64(0), 512, 512, 512},
		{"unknown kernel keeps inherited cap", ^uint64(0), 0, 1024, 524288},
	} {
		t.Run(tc.name, func(t *testing.T) {
			soft, hard := userDescriptorCeiling(tc.inherited, tc.kernel)
			if soft != tc.wantSoft || hard != tc.wantHard {
				t.Fatalf("userDescriptorCeiling(%d, %d) = %d:%d, want %d:%d", tc.inherited, tc.kernel, soft, hard, tc.wantSoft, tc.wantHard)
			}
			if soft > hard {
				t.Fatalf("soft %d exceeds hard %d", soft, hard)
			}
		})
	}
}

func TestUserPlatformPreflightIsInstalledOnlyWhereThePlatformNeedsIt(t *testing.T) {
	if userPlatformPreflight == nil {
		t.Fatal("userPlatformPreflight must never be nil; inspection calls it unconditionally")
	}
}
