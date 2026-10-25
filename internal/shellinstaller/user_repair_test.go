package shellinstaller

import (
	"errors"
	"strings"
	"testing"
)

func TestUserSeparateRepairPreservesCauseAndOldData(t *testing.T) {
	cause := errors.New("graph drift")
	err := userGraphRepairError("/owned/damaged", "separate", cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "different empty target") || !strings.Contains(err.Error(), "gentle-ai shell install --help") || !strings.Contains(err.Error(), "/owned/damaged") {
		t.Fatalf("separate repair hides cause or lacks safe continuation: %v", err)
	}
	if userGraphRepairError("/owned/shared", "shared", cause) != cause || userGraphRepairError("/owned/separate", "separate", nil) != nil {
		t.Fatal("repair advice changed shared or successful outcomes")
	}
}
