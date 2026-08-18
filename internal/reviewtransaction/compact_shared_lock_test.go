package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This file pins the read geometry behind the shared store lock's holder
// record (issue #3342 follow-up). Naming the holder is the whole point of the
// retry-safe live-contention denial, and it degraded to the anonymous
// "a concurrent review operation holds the store lock" on Windows only: its
// byte-range lock is MANDATORY, so while a holder is live every read
// overlapping the locked coordination byte fails from any other handle, even
// inside the locking process. POSIX flock is advisory and never denies the
// same read, so the platform difference has to be simulated to stay verifiable
// without a Windows runner.

func testStoreLockOwnerDocument(t *testing.T, pid int, host string) []byte {
	t.Helper()
	payload, err := json.Marshal(storeLockOwner{
		Schema: storeLockSchema, OwnerID: "deadbeefdeadbeefdeadbeefdeadbeef",
		PID: pid, Host: host, AcquiredAt: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(payload, '\n')
}

// mandatoryCoordinationLockReaderAt mimics a live Windows holder: reads that
// overlap the locked coordination byte fail, reads past it succeed.
type mandatoryCoordinationLockReaderAt struct{ payload []byte }

func (reader mandatoryCoordinationLockReaderAt) ReadAt(buffer []byte, offset int64) (int, error) {
	if offset < storeLockCoordinationBytes {
		return 0, errors.New("read LOCK: the process cannot access the file because another process has locked a portion of the file")
	}
	return bytes.NewReader(reader.payload).ReadAt(buffer, offset)
}

func TestStoreLockOwnerRecordSurvivesMandatoryCoordinationByteLock(t *testing.T) {
	document := testStoreLockOwnerDocument(t, 7700, "windows-runner")

	t.Run("advisory-lock platforms read the record whole", func(t *testing.T) {
		owner, recorded := readStoreLockOwnerRecord(bytes.NewReader(document))
		if !recorded || owner.PID != 7700 || owner.Host != "windows-runner" {
			t.Fatalf("owner record = %#v, recorded=%v; want pid 7700 on windows-runner", owner, recorded)
		}
	})

	t.Run("a denied coordination byte still names the holder", func(t *testing.T) {
		owner, recorded := readStoreLockOwnerRecord(mandatoryCoordinationLockReaderAt{payload: document})
		if !recorded || owner.PID != 7700 || owner.Host != "windows-runner" {
			t.Fatalf("owner record = %#v, recorded=%v; want pid 7700 on windows-runner", owner, recorded)
		}
	})

	t.Run("a denied coordination byte invents no holder for unreadable residue", func(t *testing.T) {
		owner, recorded := readStoreLockOwnerRecord(mandatoryCoordinationLockReaderAt{payload: []byte("not-json\n")})
		if recorded {
			t.Fatalf("unreadable residue recorded a holder: %#v", owner)
		}
	})
}

func TestDecodeStoreLockOwnerRecordAcceptsOnlyValidatedDocuments(t *testing.T) {
	document := string(testStoreLockOwnerDocument(t, 4242, "host-a"))
	// Interrupted-rewrite residue: the owner document is intact, but the file
	// no longer parses as exactly one document.
	residue := document + "{\n"

	for _, testCase := range []struct {
		name     string
		payload  string
		offset   int64
		recorded bool
		pid      int
	}{
		{name: "whole file", payload: document, offset: 0, recorded: true, pid: 4242},
		{name: "whole file with interrupted residue", payload: residue, offset: 0, recorded: true, pid: 4242},
		{name: "read past the coordination byte", payload: document[1:], offset: storeLockCoordinationBytes, recorded: true, pid: 4242},
		{name: "read past the coordination byte with residue", payload: residue[1:], offset: storeLockCoordinationBytes, recorded: true, pid: 4242},
		{name: "brace restoration never applies to a whole-file read", payload: document[1:], offset: 0},
		{name: "unreadable residue", payload: "not-json\n", offset: storeLockCoordinationBytes},
		{name: "empty lock file", payload: "", offset: storeLockCoordinationBytes},
		{name: "foreign schema", payload: `{"schema":"other/v1","pid":9,"host":"host-a"}`, offset: 0},
		{name: "absent pid", payload: `{"schema":"` + storeLockSchema + `","pid":0,"host":"host-a"}`, offset: 0},
		{name: "absent host", payload: `{"schema":"` + storeLockSchema + `","pid":9,"host":"  "}`, offset: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			owner, recorded := decodeStoreLockOwnerRecord([]byte(testCase.payload), testCase.offset)
			if recorded != testCase.recorded {
				t.Fatalf("recorded = %v, want %v (owner %#v)", recorded, testCase.recorded, owner)
			}
			if recorded && owner.PID != testCase.pid {
				t.Fatalf("owner pid = %d, want %d", owner.PID, testCase.pid)
			}
		})
	}
}

// TestAcquiredStoreLockNamesItsHolderToAnotherHandle is the end-to-end pin:
// on Windows it exercises the real mandatory byte-range lock, on POSIX the
// advisory one, and both must name the live holder. It also pins the writer
// invariant the restoration depends on -- the owner document really does open
// with storeLockOwnerDocumentPrefix -- against the bytes the writer produces.
func TestAcquiredStoreLockNamesItsHolderToAnotherHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "LOCK")
	lock, err := AcquireAuthorityFileLock(path)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			_ = lock.Release()
		}
	}()

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	owner, recorded := readStoreLockOwnerRecord(file)
	if !recorded || owner.PID != os.Getpid() {
		t.Fatalf("live holder record = %#v, recorded=%v; want pid %d", owner, recorded, os.Getpid())
	}

	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	released = true
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 || written[0] != storeLockOwnerDocumentPrefix {
		t.Fatalf("owner record does not open with %q: %q", storeLockOwnerDocumentPrefix, string(written))
	}
}
