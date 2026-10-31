//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserNativeIntegrationRetainsQualifiedMainWorker(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		for _, background := range []string{"on", "off"} {
			for _, source := range []string{"", "stable", "main-preview"} {
				t.Run(persona+"/"+background+"/"+source, func(t *testing.T) {
					profile := UserExperience{Persona: persona, BackgroundSubagents: background, SDKSource: source}
					original := profile
					root := installStagePath(t)
					req := UserInstallRequest{Destination: root, Mode: "separate", Experience: &profile}
					token, err := InspectUserInstall(req)
					if err != nil {
						t.Fatal(err)
					}
					encoded, err := profile.Encode()
					if err != nil {
						t.Fatal(err)
					}
					before, err := userTreeStamp(filepath.Dir(root))
					if err != nil {
						t.Fatal(err)
					}
					args := []string{root, "separate", "", "", token, "--experience", encoded}
					selected, native, err := userNativeFreshRequest(args)
					if err != nil || native != (source != "main-preview") || selected.Experience == nil || *selected.Experience != original || profile != original {
						t.Fatal("dispatcher changed the full consent-bound selection or producer", selected, native, err)
					}
					if after, err := userTreeStamp(filepath.Dir(root)); err != nil || after != before {
						t.Fatal("producer selection prepared filesystem state", err)
					}
					if source == "main-preview" {
						if err := userValidateStageExperience(selected.Experience); err == nil {
							t.Fatal("generic native writer acquired unsupported Main authority")
						}
						// The original, typed Main stage consumes this complete profile.
						// No SDK selector is cleared to manufacture a generic consumer.
						stage := installStagePath(t)
						if err := userPrepareInstallStage(context.Background(), stage, "separate", selected.Experience); err != nil {
							t.Fatal("qualified Main producer no longer receives its profile", err)
						}
						if *selected.Experience != original {
							t.Fatal("typed Main stage changed original preferences")
						}
					}
					args[4] = strings.Repeat("0", 64)
					if _, _, err := userNativeFreshRequest(args); err == nil {
						t.Fatal("producer dispatch accepted stale physical consent")
					}
				})
			}
		}
	}
}

func TestUserNativeIntegrationRejectsMalformedSelection(t *testing.T) {
	for _, args := range [][]string{nil, {"missing"}, {"/inert", "separate", "", "", "token", "--experience", "not-canonical"}} {
		if _, native, err := userNativeFreshRequest(args); err == nil || native {
			t.Fatal("malformed entry became a legacy or native selection", args, native, err)
		}
	}
}

func TestUserNativeIntegrationPreservesStoppedContextCauses(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if cause == context.DeadlineExceeded {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(1, 0))
			} else {
				cancel()
			}
			defer cancel()
			parent := filepath.Dir(installStagePath(t))
			before, err := userTreeStamp(parent)
			if err != nil {
				t.Fatal(err)
			}
			var group userNativeSlice
			calls := map[string]func() error{
				"install": func() error {
					return userNativeInstall(ctx, "", nil, UserInstallRequest{Destination: filepath.Join(parent, "final")}, nil, io.Discard, io.Discard)
				},
				"coordinate":  func() error { return userNativeCoordinate(ctx, "", make([]string, 8), nil, io.Discard, io.Discard) },
				"owner-entry": func() error { return userNativeOwnerEntry(ctx, "", nil, nil, io.Discard, io.Discard) },
				"owner-replace": func() error {
					return userReplaceOwner(ctx, "", []string{"install"}, os.Stdin, os.Stdout, os.Stderr, func(string, []string, []string) error {
						t.Fatal("stopped owner crossed replacement boundary")
						return nil
					})
				},
				"slice-argument": func() error { _, err := group.serviceArg(ctx); return err },
				"slice-member":   func() error { return group.member(ctx) },
				"service-plan":   func() error { _, err := newUserNativeServicePlan(ctx, &group, nil); return err },
				"pnpm-readback": func() error {
					_, err := ReadNativePNPMInstallation(ctx, parent, NativeInstallationLocation{})
					return err
				},
			}
			for name, call := range calls {
				if err := call(); !errors.Is(err, cause) {
					t.Errorf("%s lost actual stopped-context cause: %v", name, err)
				}
			}
			if after, err := userTreeStamp(parent); err != nil || before != after {
				t.Fatal("stopped native boundary changed filesystem preimage", err)
			}
		})
	}
	if _, err := ReadNativePNPMInstallation(nil, "/inert", NativeInstallationLocation{}); err == nil {
		t.Fatal("nil readback context accepted")
	}
}

func TestUserNativeIntegrationRefusesNonPrivateProducerParent(t *testing.T) {
	for _, persona := range []string{"gentleman", "neutral", "unmanaged"} {
		t.Run(persona, func(t *testing.T) {
			parent := t.TempDir()
			if err := os.Chmod(parent, 0755); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(parent)
			if err != nil {
				t.Fatal(err)
			}
			profile := UserExperience{Persona: persona, BackgroundSubagents: "off"}
			original := profile
			root := filepath.Join(parent, "producer")
			err = userPrepareNativeProducer(context.Background(), root, &profile)
			var refusal *PrivateRuntimeError
			if !errors.As(err, &refusal) || refusal.Kind != "refused" || !strings.Contains(err.Error(), "filesystem") || profile != original {
				t.Fatal("nonprivate parent bypassed admission or changed profile", err)
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("refused producer created destination", err)
			}
			after, err := os.Lstat(parent)
			if err != nil || privateStamp(before) != privateStamp(after) {
				t.Fatal("refused producer changed parent custody", err)
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("refused producer wrote before admission", entries, err)
			}
		})
	}
}

type nativeIntegrationReadCloser struct {
	io.Reader
	closeErr error
	closed   bool
}

func (r *nativeIntegrationReadCloser) Close() error {
	r.closed = true
	return r.closeErr
}

func TestUserNativeIntegrationSmallReadOwnsCloseFailure(t *testing.T) {
	cause := errors.New("owned PNPM reader close failed")
	reader := &nativeIntegrationReadCloser{Reader: strings.NewReader("fixture DATA"), closeErr: cause}
	if data, err := userPNPMReadSmall(reader); !errors.Is(err, cause) || data != nil || !reader.closed {
		t.Fatal("close failure yielded usable data or lost ownership", data, err, reader.closed)
	}
	for _, size := range []int{65536, 65537} {
		reader := &nativeIntegrationReadCloser{Reader: strings.NewReader(strings.Repeat("x", size))}
		data, err := userPNPMReadSmall(reader)
		if !reader.closed || (size == 65536 && (err != nil || len(data) != size)) || (size > 65536 && (err == nil || data != nil)) {
			t.Fatal("existing small-read bound or close ownership changed", size, len(data), err, reader.closed)
		}
	}
}
