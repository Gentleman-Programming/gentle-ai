//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUserNativeChildDeadline(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, tt := range []struct {
		name, value string
		refuse      bool
	}{
		{"remaining operation", strconv.FormatInt(now.Add(time.Second).UnixNano(), 10), false},
		{"maximum", strconv.FormatInt(now.Add(140*time.Second).UnixNano(), 10), false},
		{"expired", strconv.FormatInt(now.UnixNano(), 10), true},
		{"extended", strconv.FormatInt(now.Add(141*time.Second).UnixNano(), 10), true},
		{"malformed", "not-a-deadline", true},
		{"overflow", "9223372036854775808", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			deadline, err := userNativeDeadline(tt.value, now)
			if (err != nil) != tt.refuse {
				t.Fatal(deadline, err)
			}
		})
	}
}

func TestUserNativeChildRefusal(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "preserve")
	if err := os.WriteFile(marker, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{root, filepath.Join(root, "stage"), "bad", filepath.Join(root, "destination"), "separate", "", "", "unconfirmed"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tt := range []struct {
		name string
		ctx  context.Context
		args []string
	}{
		{"missing arguments", context.Background(), nil},
		{"nil context", nil, args},
		{"canceled", canceled, args},
		{"invalid deadline", context.Background(), args},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := userNativeChild(tt.ctx, tt.args, &output); err == nil || output.Len() != 0 {
				t.Fatal("refused entry reported success", err, output.String())
			}
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "unchanged" {
				t.Fatal("refusal changed fixture", err)
			}
		})
	}
}

func TestUserNativeOperation(t *testing.T) {
	phase := os.Getenv("GENTLE_PRIVATE_GUARD_TEST")
	slice, err := userNativeOperation(context.Background())
	if phase != "sealed" && phase != "plain-rw" {
		if err == nil {
			t.Fatal("ordinary worker accepted without an operation slice", slice)
		}
		return
	}
	if err != nil {
		t.Fatal("actual operation member refused", err)
	}
	if err := slice.member(context.Background()); err != nil {
		t.Fatal(err)
	}
	argument, err := slice.serviceArg(context.Background())
	if err != nil || argument != "--slice="+slice.name {
		t.Fatal(argument, err)
	}
}

func TestUserNativeChildReentry(t *testing.T) {
	kernelErr := UserKernelCheck()
	var output bytes.Buffer
	err := RunUserEntry(context.Background(), "", []string{"internal-native-produce"}, nil, &output, &output)
	if kernelErr != nil {
		if err == nil || err.Error() != kernelErr.Error() || output.Len() != 0 {
			t.Fatal("unqualified internal selector bypassed kernel admission", err, kernelErr, output.String())
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "invalid native child arguments") || output.Len() != 0 {
		t.Fatal("internal selector did not reach guarded child entry", err, output.String())
	}
}

func TestUserNativeChildMalformedArguments(t *testing.T) {
	var output bytes.Buffer
	err := userNativeChild(context.Background(), nil, &output)
	if err == nil || !strings.Contains(err.Error(), "invalid native child arguments") || output.Len() != 0 {
		t.Fatal("malformed child arguments accepted before admission", err, output.String())
	}
}
