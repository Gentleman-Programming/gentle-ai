package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// freshJSON re-encodes the embedded list with one mutation applied.
func freshJSON(base []byte, mutate func(*userReviewedList)) ([]byte, error) {
	var list userReviewedList
	if err := json.Unmarshal(base, &list); err != nil {
		return nil, err
	}
	mutate(&list)
	return json.Marshal(list)
}

// userReviewedListResponse is a canned transport response: responses are
// produced in-process, so no loader test ever touches the network.
type userReviewedListResponse struct {
	status  int
	headers map[string]string
	body    []byte
	length  int64 // Declared Content-Length; len(body) when zero.
	err     error
}

func (response userReviewedListResponse) RoundTrip(*http.Request) (*http.Response, error) {
	if response.err != nil {
		return nil, response.err
	}
	headers := http.Header{}
	for key, value := range response.headers {
		headers.Set(key, value)
	}
	length := response.length
	if length == 0 {
		length = int64(len(response.body))
	}
	return &http.Response{
		StatusCode:    response.status,
		Status:        http.StatusText(response.status),
		Header:        headers,
		ContentLength: length,
		Body:          io.NopCloser(bytes.NewReader(response.body)),
	}, nil
}

func userReviewedListClientStub(response userReviewedListResponse) *http.Client {
	return &http.Client{Transport: response}
}

func userEmbeddedListBytes(t *testing.T) []byte {
	t.Helper()
	_, data, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The embedded copy parses strictly and carries the reviewed releases: the
// offline fallback and the frozen lock's supplier authority.
func TestUserEmbeddedReviewedList(t *testing.T) {
	list, data, err := userEmbeddedReviewedList()
	if err != nil {
		t.Fatal(err)
	}
	if list.Schema != userReviewedListSchema {
		t.Fatalf("embedded schema differs: %q", list.Schema)
	}
	if len(list.Releases) != 3 {
		t.Fatalf("embedded release set differs: %v", list.Releases)
	}
	if newest := list.newestReviewedRelease(); newest != "4.5.0" {
		t.Fatalf("newest reviewed release differs: %q", newest)
	}
	entry, ok := list.Releases["4.3.0"]
	if !ok || entry.Installer != "8679f49d3675323bf7ff5c24321f08016bb0f8e79ded307b5900c37648f52470" ||
		entry.Binary != "61bb8e7e23864a15243882fc029f25f64118d593088cd7f64c3bb519dcf80b80" ||
		!userReviewedListSRI(entry.Integrity) {
		t.Fatalf("embedded 4.3.0 supplier entry differs: %+v", entry)
	}
	file, err := os.ReadFile("../../scripts/gentle-pi-suppliers.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file, data) {
		t.Fatal("embedded reviewed list differs from the repository file")
	}
}

func TestUserParseReviewedListRefusals(t *testing.T) {
	base := userEmbeddedListBytes(t)
	mutations := map[string]func(*userReviewedList){
		"foreign-schema": func(list *userReviewedList) { list.Schema = "other/v1" },
		"empty-schema":   func(list *userReviewedList) { list.Schema = "" },
		"empty-releases": func(list *userReviewedList) { list.Releases = map[string]userReviewedSupplier{} },
		"range-key": func(list *userReviewedList) {
			list.Releases["^4.6.0"] = list.Releases["4.5.0"]
			delete(list.Releases, "4.5.0")
		},
		"prerelease-key": func(list *userReviewedList) {
			list.Releases["4.6.0-beta.1"] = list.Releases["4.5.0"]
			delete(list.Releases, "4.5.0")
		},
		"leading-zero-key": func(list *userReviewedList) {
			list.Releases["04.5.0"] = list.Releases["4.5.0"]
			delete(list.Releases, "4.5.0")
		},
		"wrong-sri-algorithm": func(list *userReviewedList) {
			list.Releases["4.5.0"] = userReviewedSupplier{Integrity: "sha1-x", Installer: strings.Repeat("a", 64), Binary: strings.Repeat("b", 64)}
		},
		"nonroundtrip-sri": func(list *userReviewedList) {
			value := list.Releases["4.5.0"].Integrity
			list.Releases["4.5.0"] = userReviewedSupplier{Integrity: value[:94] + "B" + value[95:], Installer: list.Releases["4.5.0"].Installer, Binary: list.Releases["4.5.0"].Binary}
		},
		"uppercase-digest": func(list *userReviewedList) {
			list.Releases["4.5.0"] = userReviewedSupplier{Integrity: list.Releases["4.5.0"].Integrity, Installer: strings.Repeat("A", 64), Binary: list.Releases["4.5.0"].Binary}
		},
		"short-digest": func(list *userReviewedList) {
			list.Releases["4.5.0"] = userReviewedSupplier{Integrity: list.Releases["4.5.0"].Integrity, Installer: list.Releases["4.5.0"].Installer, Binary: "abc"}
		},
		"missing-entry-values": func(list *userReviewedList) { list.Releases["4.5.0"] = userReviewedSupplier{} },
	}
	for name, mutate := range mutations {
		raw, err := freshJSON(base, mutate)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := userParseReviewedList(raw); err == nil {
			t.Fatalf("%s list accepted", name)
		}
	}
	for name, raw := range map[string]string{
		"not-json":        `{"schema":`,
		"array-root":      `[]`,
		"unknown-field":   `{"schema":"gentle-shell-reviewed-suppliers/v1","releases":{"4.5.0":{"integrity":"sha512-x","installer":"x","binary":"x"}},"foreign":1}`,
		"unknown-entry":   `{"schema":"gentle-shell-reviewed-suppliers/v1","releases":{"4.5.0":{"integrity":"i","installer":"a","binary":"b","foreign":1}}}`,
		"trailing-object": `{"schema":"gentle-shell-reviewed-suppliers/v1","releases":{"4.5.0":{"integrity":"i","installer":"a","binary":"b"}}} {}`,
	} {
		if _, err := userParseReviewedList([]byte(raw)); err == nil {
			t.Fatalf("%s list accepted", name)
		}
	}
}

// Semver ordering picks the newest listed release: 4.10.0 above 4.9.0.
func TestUserReviewedListNewestRelease(t *testing.T) {
	list := userReviewedList{Releases: map[string]userReviewedSupplier{
		"4.9.0": {}, "4.10.0": {}, "4.2.0": {},
	}}
	if newest := list.newestReviewedRelease(); newest != "4.10.0" {
		t.Fatalf("newest release differs: %q", newest)
	}
	if newest := (userReviewedList{}).newestReviewedRelease(); newest != "" {
		t.Fatalf("empty list newest differs: %q", newest)
	}
}

// The semver comparator is a lexicographic (major, minor, patch) tuple
// comparison: 4.5.1 sorts below 4.6.0 and 4.9.0 below 4.10.0, so
// newestReviewedRelease is deterministic across map iteration orders.
func TestUserReviewedListVersionLessOrdering(t *testing.T) {
	cases := []struct {
		left, right string
		less        bool
	}{
		{"4.5.1", "4.6.0", true},
		{"4.6.0", "4.5.1", false},
		{"4.9.0", "4.10.0", true},
		{"4.10.0", "4.9.0", false},
		{"4.5.0", "4.5.0", false},
		{"4.5.1", "4.5.10", true},
		{"3.9.9", "4.0.0", true},
	}
	for _, item := range cases {
		if got := userReviewedListVersionLess(item.left, item.right); got != item.less {
			t.Fatalf("userReviewedListVersionLess(%q, %q) = %v, want %v", item.left, item.right, got, item.less)
		}
	}
	list := userReviewedList{Releases: map[string]userReviewedSupplier{"4.5.1": {}, "4.6.0": {}}}
	for i := 0; i < 10; i++ {
		if newest := list.newestReviewedRelease(); newest != "4.6.0" {
			t.Fatalf("newest reviewed release not deterministic on pass %d: %q", i, newest)
		}
	}
}

// The live list wins when its fetch and strict validation succeed; the
// selected bytes are exactly the live response, so the stage and detection use
// one authority.
func TestUserSelectedReviewedListPrefersLive(t *testing.T) {
	embedded := userEmbeddedListBytes(t)
	live := bytes.Replace(embedded, []byte(`"4.5.0"`), []byte(`"4.6.0"`), 1)
	if bytes.Equal(live, embedded) {
		t.Fatal("live fixture construction differs")
	}
	list, selected, err := userSelectedReviewedListUsing(context.Background(), userReviewedListClientStub(userReviewedListResponse{status: 200, body: live}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(selected, live) {
		t.Fatal("selected bytes differ from the live list")
	}
	if newest := list.newestReviewedRelease(); newest != "4.6.0" {
		t.Fatalf("live list target differs: %q", newest)
	}
}

// Any fetch or validation failure falls back to the embedded copy and is
// never an error for the user.
func TestUserSelectedReviewedListFallsBack(t *testing.T) {
	embedded := userEmbeddedListBytes(t)
	for name, response := range map[string]userReviewedListResponse{
		"transport-error":  {err: errors.New("offline")},
		"http-error":       {status: 404, body: []byte("missing")},
		"oversized-header": {status: 200, length: 99999999, body: []byte("{})")},
		"gzipped":          {status: 200, headers: map[string]string{"Content-Encoding": "gzip"}, body: []byte("gzip")},
		"invalid-json":     {status: 200, body: []byte("not json")},
		"invalid-schema":   {status: 200, body: []byte(`{"schema":"other/v1","releases":{}}`)},
	} {
		list, selected, err := userSelectedReviewedListUsing(context.Background(), userReviewedListClientStub(response))
		if err != nil {
			t.Fatalf("%s fetch surfaced as an error: %v", name, err)
		}
		if !bytes.Equal(selected, embedded) {
			t.Fatalf("%s fetch did not fall back to the embedded bytes", name)
		}
		if newest := list.newestReviewedRelease(); newest != "4.5.0" {
			t.Fatalf("%s fallback target differs: %q", name, newest)
		}
	}
}

// The bounded client refuses an oversized body even without a length header.
func TestUserReviewedListBodyBound(t *testing.T) {
	huge := bytes.Repeat([]byte("a"), userReviewedListBound+2)
	client := &http.Client{Transport: userReviewedListTransportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(huge))}, nil
	})}
	if _, err := userReviewedListUsing(context.Background(), client); err == nil {
		t.Fatal("oversized live body accepted")
	}
}

type userReviewedListTransportFunc func(*http.Request) (*http.Response, error)

func (fn userReviewedListTransportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// The fixed publisher URL: no other endpoint is ever read.
func TestUserReviewedListURL(t *testing.T) {
	var observed string
	client := &http.Client{Transport: userReviewedListTransportFunc(func(request *http.Request) (*http.Response, error) {
		observed = request.URL.String()
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(userEmbeddedListBytes(t)))}, nil
	})}
	if _, _, err := userSelectedReviewedListUsing(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if observed != userReviewedListURL {
		t.Fatalf("live list URL differs: %q", observed)
	}
}

// A caller deadline is never an error: an expired or exceeded deadline falls
// back to the embedded copy like any other fetch failure, so the TUI check
// stays inside its budget and still resolves a target.
func TestUserSelectedReviewedListDeadlineFallsBack(t *testing.T) {
	embedded := userEmbeddedListBytes(t)
	ctx, cancel := context.WithTimeout(context.Background(), -time.Second)
	defer cancel()
	list, selected, err := userSelectedReviewedListUsing(ctx, userReviewedListClientStub(userReviewedListResponse{status: 200, body: []byte("late")}))
	if err != nil {
		t.Fatalf("caller deadline surfaced as an error: %v", err)
	}
	if !bytes.Equal(selected, embedded) {
		t.Fatal("caller deadline did not fall back to the embedded bytes")
	}
	if newest := list.newestReviewedRelease(); newest != "4.5.0" {
		t.Fatalf("deadline fallback target differs: %q", newest)
	}
}

// The bounded client never outlives the caller's remaining deadline and never
// exceeds the 10 s budget, well inside the TUI check's 20 s window.
func TestUserReviewedListClientDeadlineBound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if client := userReviewedListClient(ctx); client.Timeout <= 0 || client.Timeout > 2*time.Second {
		t.Fatalf("client timeout not bounded by the caller deadline: %v", client.Timeout)
	}
	if client := userReviewedListClient(context.Background()); client.Timeout != 10*time.Second {
		t.Fatalf("client timeout differs from the 10 s budget: %v", client.Timeout)
	}
}

// Canceled contexts propagate before any response is trusted.
func TestUserReviewedListCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := userSelectedReviewedListUsing(ctx, userReviewedListClientStub(userReviewedListResponse{status: 200, body: userEmbeddedListBytes(t)})); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context not propagated: %v", err)
	}
}

// The installed version comes from installation.json, which the update check
// reads without semver validation. A malformed value must count as unrecorded
// (offer the update once) instead of panicking the check.
func TestUserReviewedVersionOlderMalformedInstalled(t *testing.T) {
	for _, installed := range []string{"4", "4.5", "abc", "4.5.x", "-1.0.0", "4.5.0.1"} {
		older := func() (older bool) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("installed %q panicked: %v", installed, recovered)
				}
			}()
			return userReviewedVersionOlder(installed, "4.5.0")
		}()
		if !older {
			t.Fatalf("malformed installed %q must count as unrecorded (outdated)", installed)
		}
	}
	if userReviewedVersionOlder("4.5.0", "4.5.0") || !userReviewedVersionOlder("4.4.0", "4.5.0") || userReviewedVersionOlder("4.6.0", "4.5.0") {
		t.Fatal("well-formed comparison changed")
	}
}
