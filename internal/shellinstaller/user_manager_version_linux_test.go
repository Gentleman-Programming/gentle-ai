//go:build linux

package shellinstaller

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestUserManagerProbeReadsManagerVersion(t *testing.T) {
	want := []string{"/usr/bin/systemctl", "--user", "show", "--property=Version", "--value"}
	if got := userManagerProbe(context.Background()).Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("client version is not manager evidence: %v", got)
	}
}

func TestUserManagerVersionAcceptsPackagedPrerelease(t *testing.T) {
	for _, value := range []string{"254", "256~rc1", "254.7-1ubuntu8.8", "259.5-0ubuntu3.4\n"} {
		t.Run(strings.TrimSpace(value), func(t *testing.T) {
			if err := userManagerVersion([]byte(value)); err != nil {
				t.Fatalf("supported manager version refused: %v", err)
			}
		})
	}
	for _, value := range []string{"253", "", "systemd 256", "x256", "256\nrc1", "254.", "254;command", strings.Repeat("2", 4097)} {
		if err := userManagerVersion([]byte(value)); err == nil {
			t.Fatalf("unsupported or malformed version admitted: %.40q", value)
		}
	}
}

func TestUserServiceDescriptionDoesNotUsePiArguments(t *testing.T) {
	args := userServiceArgs("owned-unit", false, "/owned/supervisor", "/owned/project", []string{"launch", "private-pi-argument"}, nil)
	var descriptions []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--description=") {
			descriptions = append(descriptions, arg)
		}
	}
	if !reflect.DeepEqual(descriptions, []string{"--description=Gentle Shell owned runtime"}) {
		t.Fatalf("fixed description missing or contains Pi arguments: %v", descriptions)
	}
}
