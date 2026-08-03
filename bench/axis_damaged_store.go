package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// This file is ONE opt-in axis, and deleting it is a supported operation. The
// core corpus does not reference anything in here; `go build`, `go vet`,
// `go test` and `gentle-ai-bench run` all work with this file absent, and the
// numbers they produce are unchanged. That is the test of whether the seam in
// axis.go is real, and it is worth re-running whenever this file grows.
//
// ---------------------------------------------------------------------------
// Why the axis exists
// ---------------------------------------------------------------------------
//
// Every journey in the core corpus starts from an empty directory and builds
// its state by running commands. That is what makes the friction numbers
// honest, and it has a cost nobody had named: a black-box harness can only
// visit states the product agrees to construct.
//
// A community tester reached a state the product refuses to create.
// `validateCompactRecoveryEdge` runs at write time on both `review recover` and
// the compact transport import, so no sequence of CLI commands produces a store
// holding a recovery edge that does not re-derive. Reproducing it needed store
// bytes written directly — and once reproduced it turned out to be a dead end:
// the store reports itself non-authoritative, `review inspect-authority`
// publishes `anomaly_classes: []`, and no advertised recovery surface admits
// the edges.
//
// Real repositories reach states like that through history. A store written by
// an older build, an operation interrupted between two writes, a revision that
// drifted while something else moved. Ours never do, because ours are minutes
// old.
//
// ---------------------------------------------------------------------------
// What it costs, and how that cost is contained
// ---------------------------------------------------------------------------
//
// These fixtures author `review-state.json` on disk. That breaks the black-box
// property, and it makes the journeys coupled to a persisted format instead of
// to a CLI. The failure mode is specific and nasty: the format moves, the bytes
// stop producing the state the journey claims, and the journey keeps passing
// while measuring nothing.
//
// Three things contain it, in order of how much work they do:
//
//  1. Every fixture reads its damage back OUT of the product and requires the
//     product to report exactly the damage the journey claims, before the
//     journey is allowed to spend a single counted command. A fixture whose
//     bytes stopped producing the intended state fails the run loudly.
//
//  2. Before any edit, loadStoreRecord re-derives the record's own revision
//     from the bytes it just read and requires it to equal the recorded one.
//     The product's revision is a SHA-256 over the canonical marshalling of the
//     state, so reproducing it is proof that this file can still write bytes
//     the product will accept. When the marshalling moves, that check fails
//     first and names the reason, instead of a fixture silently writing a store
//     the product then rejects as a checksum mismatch — which would look like a
//     completely different defect.
//
//  3. Every step declares its capability, so a build lacking one of these verbs
//     records `unsupported` rather than a pass or a failure.
//
// The layout below is deliberately derived from a store built through the CLI
// in the fixture itself, never from the product's Go structs: what these
// journeys depend on is the persisted format, and depending on it directly is
// what makes the drift visible.

func init() {
	RegisterAxis(Axis{
		Name:     damagedStoreAxis,
		Title:    "Journeys that start from a compact-v2 review store already damaged on disk",
		BlackBox: false,
		Properties: []string{
			"Fixtures author `review-state.json` directly. The core corpus reaches every state it measures through the CLI; these states cannot be reached that way, because the product validates the recovery edge at write time and refuses to create them.",
			"Because they are coupled to a persisted format rather than to a CLI, these journeys are NOT portable across builds the way the core is. Against a build whose store format has moved they report `failed` or `unsupported`; they will not report a clean number for a state they no longer built.",
			"Each fixture proves its damage by reading it back through the product — `review inspect-authority` or `review status` — and requires the exact shape the journey claims before any counted command runs.",
			"Each fixture also re-derives the store's own revision from the bytes it read, before editing them. That check is the format-drift tripwire: it fails first, and names the reason, instead of letting a fixture write a store the product rejects for an unrelated-looking reason.",
			"The states here are reached by history in a real repository — an older build, an interrupted write, a revision that drifted — and by nothing an operator types.",
		},
		Journeys: damagedStoreJourneys,
	})
}

const damagedStoreAxis = "damaged-store"

// ---------------------------------------------------------------------------
// The persisted record, read and written without a struct
// ---------------------------------------------------------------------------
//
// The product's revision binds the exact canonical marshalling of the state
// object, which is field-declaration ordered and HTML-escaped. Decoding into a
// Go map would sort the keys and decoding into a struct would require this file
// to mirror internal types — the thing it must not do. So a record is decoded
// into an ordered tree that preserves key order and numeric literals verbatim,
// edited in place, and re-emitted with the same rules the product marshals by.
//
// Whether that reproduction is still exact is not assumed: loadStoreRecord
// checks it against the revision the product itself wrote.

type jsonMember struct {
	Key   string
	Value any
}

type jsonObject []jsonMember

type jsonArray []any

func decodeOrderedJSON(payload []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	value, err := decodeOrderedValue(decoder)
	if err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("more than one JSON value in the record")
	}
	return value, nil
}

func decodeOrderedValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := jsonObject{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key is not a string")
			}
			value, err := decodeOrderedValue(decoder)
			if err != nil {
				return nil, err
			}
			object = append(object, jsonMember{Key: key, Value: value})
		}
		_, err := decoder.Token()
		return object, err
	case '[':
		array := jsonArray{}
		for decoder.More() {
			value, err := decodeOrderedValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := decoder.Token()
		return array, err
	}
	return nil, fmt.Errorf("unexpected delimiter %v", delimiter)
}

// encodeOrderedJSON writes the compact form the product hashes: no whitespace,
// declaration order preserved, scalars marshalled by encoding/json itself so
// string escaping is the product's escaping and not an imitation of it.
func encodeOrderedJSON(value any, out *bytes.Buffer) error {
	switch typed := value.(type) {
	case jsonObject:
		out.WriteByte('{')
		for index, member := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := encodeOrderedJSON(member.Key, out); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := encodeOrderedJSON(member.Value, out); err != nil {
				return err
			}
		}
		out.WriteByte('}')
		return nil
	case jsonArray:
		out.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				out.WriteByte(',')
			}
			if err := encodeOrderedJSON(item, out); err != nil {
				return err
			}
		}
		out.WriteByte(']')
		return nil
	case json.Number:
		out.WriteString(typed.String())
		return nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return err
		}
		out.Write(encoded)
		return nil
	}
}

func orderedMember(value any, key string) (any, bool) {
	object, ok := value.(jsonObject)
	if !ok {
		return nil, false
	}
	for _, member := range object {
		if member.Key == key {
			return member.Value, true
		}
	}
	return nil, false
}

func orderedString(value any, key string) (string, bool) {
	member, ok := orderedMember(value, key)
	if !ok {
		return "", false
	}
	text, ok := member.(string)
	return text, ok
}

func setOrderedMember(value any, key string, next any) bool {
	object, ok := value.(jsonObject)
	if !ok {
		return false
	}
	for index := range object {
		if object[index].Key == key {
			object[index].Value = next
			return true
		}
	}
	return false
}

// storeStatePrefix is the domain separator the product hashes the state under.
// It is part of the persisted format, which is what this axis depends on.
const storeStatePrefix = "gentle-ai.review-state/v2\x00"

// storeRecord is one loaded `review-state.json`, still in the shape it was
// persisted in.
type storeRecord struct {
	path  string
	root  any
	state any
}

// loadStoreRecord reads one record AND proves this file can still reproduce the
// exact bytes the product hashed.
//
// The fidelity check is the load-bearing half. Everything else in this axis
// assumes it can write a store the product will read; the moment the persisted
// marshalling moves, that assumption is false, and a fixture that went ahead
// would write a record the product rejects as a checksum mismatch — a symptom
// that looks nothing like its cause. Failing here, by name, is the whole point.
func loadStoreRecord(path string) (*storeRecord, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root, err := decodeOrderedJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	state, ok := orderedMember(root, "state")
	if !ok {
		return nil, fmt.Errorf("%s carries no state object", filepath.Base(path))
	}
	recorded, ok := orderedString(root, "revision")
	if !ok {
		return nil, fmt.Errorf("%s carries no revision", filepath.Base(path))
	}
	record := &storeRecord{path: path, root: root, state: state}
	derived, err := record.deriveRevision()
	if err != nil {
		return nil, err
	}
	if derived != recorded {
		return nil, fmt.Errorf(
			"this axis can no longer reproduce the product's canonical state bytes: %s records revision %s and re-derives as %s. "+
				"The persisted compact-v2 marshalling has moved, so every fixture in the damaged-store axis is writing bytes the product would refuse. "+
				"Fix the reproduction in axis_damaged_store.go, or delete the axis; do not let it report numbers",
			filepath.Base(path), recorded, derived)
	}
	return record, nil
}

func (r *storeRecord) deriveRevision() (string, error) {
	var buffer bytes.Buffer
	if err := encodeOrderedJSON(r.state, &buffer); err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(storeStatePrefix), buffer.Bytes()...))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (r *storeRecord) recovery() (any, error) {
	recovery, ok := orderedMember(r.state, "recovery")
	if !ok {
		return nil, fmt.Errorf("%s holds no recovery provenance, so there is no edge to damage", filepath.Base(r.path))
	}
	return recovery, nil
}

func (r *storeRecord) recoveryString(key string) (string, error) {
	recovery, err := r.recovery()
	if err != nil {
		return "", err
	}
	value, ok := orderedString(recovery, key)
	if !ok {
		return "", fmt.Errorf("recovery provenance carries no %q", key)
	}
	return value, nil
}

func (r *storeRecord) setRecoveryString(key, value string) error {
	recovery, err := r.recovery()
	if err != nil {
		return err
	}
	if !setOrderedMember(recovery, key, value) {
		return fmt.Errorf("recovery provenance carries no %q to replace", key)
	}
	return nil
}

// save re-derives the revision over the edited state and writes the record
// back. The revision has to be recomputed, not preserved: the product verifies
// it on every load, so a record carrying a stale one is a malformed entry — a
// completely different state from the damaged edge these journeys are about.
func (r *storeRecord) save() (string, error) {
	revision, err := r.deriveRevision()
	if err != nil {
		return "", err
	}
	if !setOrderedMember(r.root, "revision", revision) {
		return "", errors.New("record carries no revision to replace")
	}
	var compact bytes.Buffer
	if err := encodeOrderedJSON(r.root, &compact); err != nil {
		return "", err
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return "", err
	}
	if err := os.WriteFile(r.path, append(indented.Bytes(), '\n'), 0o644); err != nil {
		return "", err
	}
	return revision, nil
}

// ---------------------------------------------------------------------------
// Where the store lives
// ---------------------------------------------------------------------------

// storeStatePath is derived through git rather than assumed, so a linked
// worktree or a separate git dir resolves the same way the product resolves it.
func storeStatePath(sandbox *Sandbox, lineage string) (string, error) {
	directory, err := storeLineageDir(sandbox, lineage)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "review-state.json"), nil
}

func storeLineageDir(sandbox *Sandbox, lineage string) (string, error) {
	common, err := gitCommonDir(sandbox, sandbox.Repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "gentle-ai", "review-transactions", "v2", lineage), nil
}

// ---------------------------------------------------------------------------
// Reading the product back — uncounted proofs and uncounted fixture work
// ---------------------------------------------------------------------------

// storeInspection is the subset of `review inspect-authority` these fixtures
// read. Unknown fields are ignored so an older or newer envelope still parses.
type storeInspection struct {
	Complete bool `json:"complete"`
	Valid    bool `json:"valid"`
	Totals   struct {
		CompactEntries   int `json:"compact_entries"`
		LoadedEntries    int `json:"loaded_entries"`
		Edges            int `json:"edges"`
		ValidEdges       int `json:"valid_edges"`
		InvalidEdges     int `json:"invalid_edges"`
		EntryDiagnostics int `json:"entry_diagnostics"`
	} `json:"totals"`
	Edges []struct {
		PredecessorLineageID string   `json:"predecessor_lineage_id"`
		SuccessorLineageID   string   `json:"successor_lineage_id"`
		SuccessorRevision    string   `json:"successor_revision"`
		Valid                bool     `json:"valid"`
		AnomalyClasses       []string `json:"anomaly_classes"`
		Problems             []string `json:"problems"`
	} `json:"edges"`
	EntryDiagnostics []struct {
		LineageID string `json:"lineage_id"`
		Problem   string `json:"problem"`
	} `json:"entry_diagnostics"`
}

// storeStatus is the subset of `review status` that says whether the store
// still governs anything.
type storeStatus struct {
	Complete      bool   `json:"complete"`
	Authoritative bool   `json:"authoritative"`
	Status        string `json:"status"`
	Entries       []struct {
		LineageID        string   `json:"lineage_id"`
		State            string   `json:"state"`
		Status           string   `json:"status"`
		Revision         string   `json:"revision"`
		SnapshotIdentity string   `json:"snapshot_identity"`
		Problems         []string `json:"problems"`
	} `json:"entries"`
}

func proveInspection(sandbox *Sandbox) (storeInspection, error) {
	var inspection storeInspection
	err := proveJSON(sandbox, &inspection, "review", "inspect-authority", "--cwd", sandbox.Repo)
	return inspection, err
}

func proveStoreStatus(sandbox *Sandbox) (storeStatus, error) {
	var status storeStatus
	err := proveJSON(sandbox, &status, "review", "status", "--cwd", sandbox.Repo)
	return status, err
}

// fixtureCommand runs one product invocation as fixture setup. It is uncounted
// for the same reason Sandbox.git is: the operator whose friction these
// journeys measure did not run it. They opened a repository whose store was
// already in this state; the commands that built it belong to history, not to
// the session under measurement.
//
// It fails loudly on a non-zero exit, because a fixture that half-built its
// premise and then measured the result would be worse than no journey at all.
func fixtureCommand(sandbox *Sandbox, args ...string) (Observation, error) {
	observation := sandbox.readBack(args...)
	if observation.ExitCode != 0 {
		return observation, fmt.Errorf("fixture command `%s` exited %d: %s",
			strings.Join(args, " "), observation.ExitCode, firstLine(observation.Stderr, observation.Stdout))
	}
	return observation, nil
}

// ---------------------------------------------------------------------------
// Building the healthy store the fixtures then damage
// ---------------------------------------------------------------------------

// Scratch keys the fixtures publish for the counted steps to read. Nothing in a
// counted step remembers what the fixture did; it reads these, and the fixture
// only writes a value it has already proven against the product.
const (
	scratchPredecessor         = "damaged-store/predecessor"
	scratchPredecessorRevision = "damaged-store/predecessor-revision"
	scratchSuccessor           = "damaged-store/successor"
	scratchSuccessorRevision   = "damaged-store/successor-revision"
	scratchSuccessorSnapshot   = "damaged-store/successor-snapshot"
	scratchMiddle              = "damaged-store/middle"
	scratchMiddleRevision      = "damaged-store/middle-revision"
	scratchLiveRevision        = "damaged-store/live-revision"
	scratchLiveSnapshot        = "damaged-store/live-snapshot"
)

// The successor names are the operator's to choose, so this axis chooses two
// and keeps them stable across runs.
const (
	damagedSuccessor = "review-damaged-successor"
	damagedMiddle    = "review-damaged-middle"
)

func scratchValue(sandbox *Sandbox, key string) (string, error) {
	value, ok := sandbox.Scratch[key]
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("the fixture published no %q, so this step has nothing to act on", key)
	}
	return value, nil
}

// approvedPredecessor builds, through the CLI and nothing else, one approved
// low-risk review over docs/intro.md, and publishes what it proved.
func approvedPredecessor(sandbox *Sandbox) error {
	if err := baseRepo(sandbox); err != nil {
		return err
	}
	if err := stageProse("", "intro")(sandbox); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "start", "--cwd", sandbox.Repo); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "finalize", "--cwd", sandbox.Repo); err != nil {
		return err
	}
	status, err := proveStoreStatus(sandbox)
	if err != nil {
		return err
	}
	if len(status.Entries) != 1 || status.Entries[0].State != "approved" || !status.Authoritative {
		return fmt.Errorf("fixture claims one approved authority but review status reports authoritative=%v entries=%+v",
			status.Authoritative, status.Entries)
	}
	sandbox.Scratch[scratchPredecessor] = status.Entries[0].LineageID
	sandbox.Scratch[scratchPredecessorRevision] = status.Entries[0].Revision
	return nil
}

// mintSuccessor widens the approved scope and recovers it into a successor, the
// ordinary way an operator would. The edge it creates is VALID at this point;
// damaging it is a separate step, so a fixture can never be confused about
// which of the two produced the state it measures.
func mintSuccessor(sandbox *Sandbox, widen func(*Sandbox) error, predecessorKey, predecessorRevisionKey, successor string) error {
	predecessor, err := scratchValue(sandbox, predecessorKey)
	if err != nil {
		return err
	}
	revision, err := scratchValue(sandbox, predecessorRevisionKey)
	if err != nil {
		return err
	}
	if err := widen(sandbox); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "recover", "--cwd", sandbox.Repo,
		"--predecessor-lineage", predecessor,
		"--expected-predecessor-revision", revision,
		"--successor-lineage", successor,
		"--disposition", "scope_changed"); err != nil {
		return err
	}
	inspection, err := proveInspection(sandbox)
	if err != nil {
		return err
	}
	if !inspection.Valid || inspection.Totals.ValidEdges != inspection.Totals.Edges || inspection.Totals.Edges == 0 {
		return fmt.Errorf("fixture claims a healthy recovery edge before any damage, but inspect-authority reports valid=%v totals=%+v",
			inspection.Valid, inspection.Totals)
	}
	status, err := proveStoreStatus(sandbox)
	if err != nil {
		return err
	}
	for _, entry := range status.Entries {
		if entry.LineageID == successor {
			sandbox.Scratch[scratchSuccessor] = successor
			sandbox.Scratch[scratchSuccessorRevision] = entry.Revision
			sandbox.Scratch[scratchSuccessorSnapshot] = entry.SnapshotIdentity
			return nil
		}
	}
	return fmt.Errorf("fixture claims a minted successor %q but review status does not list it", successor)
}

func widenWithProse(sandbox *Sandbox) error { return stageProse("", "widened")(sandbox) }

// widenWithCode stages code rather than prose, so the successor selects lenses
// and can then hold a captured reviewer result. That is the only difference
// between the pristine and the non-pristine journey, and it belongs in the
// fixture rather than in the story about it.
func widenWithCode(sandbox *Sandbox) error {
	path := filepath.Join(sandbox.Repo, "internal", "auth", "session.go")
	content := "package auth\n\n// CheckToken reports whether a session token is present.\nfunc CheckToken(token string) bool {\n\treturn token != \"\"\n}\n"
	if err := sandbox.write(path, content); err != nil {
		return err
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

// captureOneLensResult drives the product's own collect envelope once, so the
// successor holds a real reviewer result written by the product at the path the
// product chose. Nothing about the residue is authored here — which matters,
// because the pristineness rule the journey then measures is about exactly that
// residue.
func captureOneLensResult(sandbox *Sandbox) error {
	var envelope statusEnvelope
	if err := proveJSON(sandbox, &envelope, "review", "status", "--cwd", sandbox.Repo,
		"--contract", reviewContract, "--next-transition"); err != nil {
		return err
	}
	if envelope.NextTransition.Kind != "collect" || len(envelope.NextTransition.Collect.Inputs) == 0 ||
		envelope.NextTransition.Collect.Inputs[0].Name != "reviewer_result" {
		return fmt.Errorf("fixture needs a reviewer-result collect transition but the product published %q",
			envelope.NextTransition.Kind)
	}
	result, err := synthesizeReviewerResult(
		envelope.NextTransition.Collect.Inputs[0].ArtifactSubject.SubjectHash, envelope.paths())
	if err != nil {
		return err
	}
	path, err := writeScratch(sandbox, "damaged-store-reviewer.json", result)
	if err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "capture-result", "--cwd", sandbox.Repo,
		"--lineage", envelope.argument("lineage"),
		"--target", envelope.argument("target"),
		"--expected-revision", envelope.argument("expected-revision"),
		"--lens", envelope.argument("lens"),
		"--order", envelope.argument("order"),
		"--input", path); err != nil {
		return err
	}

	// Proof, from the filesystem the product wrote and not from the argv this
	// fixture passed: the successor now holds the reviewer-results directory
	// that `review abandon` treats as authoritative residue. Without it the
	// non-pristine journey would silently become the pristine one.
	directory, err := storeLineageDir(sandbox, envelope.argument("lineage"))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(directory, "reviewer-results"))
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("fixture claims a captured reviewer result but the successor holds no reviewer-results artifact: %v", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// The damage
// ---------------------------------------------------------------------------

// damageRecordedReason edits the reason the recovery provenance records while
// leaving the maintainer authorization alone.
//
// This is the first of the two shapes the community report describes: the
// authorization still carries the correct
// `gentle-ai.review-recovery-authorization/v1` prefix — so it still asserts
// that a maintainer bound this exact edge — but it now binds content the record
// no longer holds. In a real repository that is what an edited reason, or a
// record written by a build with a different reason-normalisation, leaves
// behind.
func damageRecordedReason(sandbox *Sandbox, lineage, suffix string) (string, error) {
	path, err := storeStatePath(sandbox, lineage)
	if err != nil {
		return "", err
	}
	record, err := loadStoreRecord(path)
	if err != nil {
		return "", err
	}
	authorization, err := record.recoveryString("maintainer_authorization")
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(authorization, damagedAuthorizationSchema) {
		return "", fmt.Errorf("fixture needs a %s authorization to damage, and %q records %q",
			damagedAuthorizationSchema, lineage, firstLine(authorization))
	}
	reason, err := record.recoveryString("reason")
	if err != nil {
		return "", err
	}
	if err := record.setRecoveryString("reason", reason+suffix); err != nil {
		return "", err
	}
	return record.save()
}

// damageAuthorizationPredecessorRevision is the second shape: the record's own
// `predecessor_revision` is moved forward to the revision the predecessor
// really holds now, while the `predecessor_revision=` line INSIDE the
// authorization stays where it was.
//
// That is ordinary drift. The predecessor's revision changes whenever the
// predecessor's state changes; a store that tracked one of those movements and
// not the other ends up with an authorization that still names the right schema
// and binds a revision that has moved on.
func damageAuthorizationPredecessorRevision(sandbox *Sandbox, lineage, observedPredecessorRevision string) (string, error) {
	path, err := storeStatePath(sandbox, lineage)
	if err != nil {
		return "", err
	}
	record, err := loadStoreRecord(path)
	if err != nil {
		return "", err
	}
	recorded, err := record.recoveryString("predecessor_revision")
	if err != nil {
		return "", err
	}
	if recorded == observedPredecessorRevision {
		return "", fmt.Errorf("fixture claims the predecessor moved, but %q already records revision %s", lineage, recorded)
	}
	authorization, err := record.recoveryString("maintainer_authorization")
	if err != nil {
		return "", err
	}
	if !strings.Contains(authorization, "predecessor_revision="+recorded) {
		return "", fmt.Errorf("fixture cannot leave the authorization behind: it does not bind predecessor_revision=%s", recorded)
	}
	if err := record.setRecoveryString("predecessor_revision", observedPredecessorRevision); err != nil {
		return "", err
	}
	return record.save()
}

const damagedAuthorizationSchema = "gentle-ai.review-recovery-authorization/v1"

// halveStateFile truncates a record to the first half of its bytes, which is
// what an interrupted write leaves behind. Nothing is re-derived: the point is
// that the record cannot be parsed at all.
func halveStateFile(sandbox *Sandbox, lineage string) error {
	path, err := storeStatePath(sandbox, lineage)
	if err != nil {
		return err
	}
	// Loading first is not needed to truncate, but it is needed to keep the
	// drift tripwire honest: if this axis can no longer read what the product
	// wrote, that has to fail here too, and not only in the journeys that edit.
	if _, err := loadStoreRecord(path); err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload[:len(payload)/2], 0o644)
}

// ---------------------------------------------------------------------------
// Proving the damage through the product
// ---------------------------------------------------------------------------

// requireInvalidEdges is the proof every edge journey runs before it is allowed
// to spend a counted command: the product itself must report exactly this many
// edges, all invalid, none carrying an anomaly class, and the store must have
// stopped being authoritative.
//
// `anomaly_classes: []` is the reported defect, not an incidental detail. It is
// how the product says "this edge does not re-derive AND it is not one of the
// two classes reconciliation knows how to quarantine", which is precisely the
// state with no advertised way out.
func requireInvalidEdges(sandbox *Sandbox, edges int, problemFragment string) error {
	inspection, err := proveInspection(sandbox)
	if err != nil {
		return err
	}
	if inspection.Totals.Edges != edges || inspection.Totals.InvalidEdges != edges || inspection.Totals.ValidEdges != 0 {
		return fmt.Errorf("fixture claims %d invalid recovery edges but inspect-authority reports %+v", edges, inspection.Totals)
	}
	if inspection.Valid {
		return errors.New("fixture claims a damaged store but inspect-authority reports the authority graph valid")
	}
	for _, edge := range inspection.Edges {
		if edge.Valid {
			return fmt.Errorf("fixture claims every edge invalid but %q re-derives", edge.SuccessorLineageID)
		}
		if len(edge.AnomalyClasses) != 0 {
			return fmt.Errorf(
				"fixture claims an edge no advertised recovery surface admits, but the product classified %q as %v; "+
					"an edge with an anomaly class is reconcilable and this journey is measuring the wrong state",
				edge.SuccessorLineageID, edge.AnomalyClasses)
		}
		if !anyContains(edge.Problems, problemFragment) {
			return fmt.Errorf("fixture claims the problem %q on %q but the product reports %v",
				problemFragment, edge.SuccessorLineageID, edge.Problems)
		}
	}
	return requireStoreNotAuthoritative(sandbox)
}

func requireStoreNotAuthoritative(sandbox *Sandbox) error {
	status, err := proveStoreStatus(sandbox)
	if err != nil {
		return err
	}
	if status.Authoritative {
		return errors.New("fixture claims a damaged store but review status still reports it authoritative")
	}
	return nil
}

func anyContains(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

// theExactBindingProblem is the wording the product emits for an authorization
// that carries the schema and binds different content. It is matched as a
// fragment rather than in full because the full message carries the target
// identity, which is a fixture-dependent hash.
const theExactBindingProblem = "exact maintainer authorization binding"

// ---------------------------------------------------------------------------
// Fixtures, whole
// ---------------------------------------------------------------------------

// damagedEdgePristine is the single-edge shape with a successor that never
// captured anything.
func damagedEdgePristine(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := mintSuccessor(sandbox, widenWithProse, scratchPredecessor, scratchPredecessorRevision, damagedSuccessor); err != nil {
		return err
	}
	revision, err := damageRecordedReason(sandbox, damagedSuccessor, " (edited after the fact)")
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchSuccessorRevision] = revision
	return requireInvalidEdges(sandbox, 1, theExactBindingProblem)
}

// damagedEdgeWithResults is the same edge under a successor that captured a
// reviewer result, so `review abandon` has residue to refuse on.
func damagedEdgeWithResults(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := mintSuccessor(sandbox, widenWithCode, scratchPredecessor, scratchPredecessorRevision, damagedSuccessor); err != nil {
		return err
	}
	if err := captureOneLensResult(sandbox); err != nil {
		return err
	}
	revision, err := damageRecordedReason(sandbox, damagedSuccessor, " (edited after the fact)")
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchSuccessorRevision] = revision
	return requireInvalidEdges(sandbox, 1, theExactBindingProblem)
}

// damagedEdgePair is the community-reported shape: two recovery edges in one
// store, each carrying a correctly-prefixed authorization that binds content
// the record no longer holds, and each reaching that state a different way.
//
// The two damages are causally linked, which is what makes the story a real
// one rather than two edits in a row. Editing the middle lineage's reason moves
// its revision, so the newest successor's recorded predecessor_revision has to
// move with it — and its authorization, written once and never rewritten, is
// left binding the revision that was current when it was minted. That is
// exactly "a stale predecessor_revision from ordinary drift".
func damagedEdgePair(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := mintSuccessor(sandbox, widenWithProse, scratchPredecessor, scratchPredecessorRevision, damagedMiddle); err != nil {
		return err
	}
	sandbox.Scratch[scratchMiddle] = damagedMiddle

	// The middle lineage has to become approved authority before it can be a
	// recovery predecessor of its own.
	if _, err := fixtureCommand(sandbox, "review", "finalize", "--cwd", sandbox.Repo); err != nil {
		return err
	}
	status, err := proveStoreStatus(sandbox)
	if err != nil {
		return err
	}
	middleRevision := ""
	for _, entry := range status.Entries {
		if entry.LineageID == damagedMiddle {
			if entry.State != "approved" {
				return fmt.Errorf("fixture claims an approved middle lineage but the product reports %q", entry.State)
			}
			middleRevision = entry.Revision
		}
	}
	if middleRevision == "" {
		return fmt.Errorf("fixture claims an approved middle lineage but review status does not list %q", damagedMiddle)
	}
	sandbox.Scratch[scratchMiddleRevision] = middleRevision

	if err := mintSuccessor(sandbox, stageProse("", "third"), scratchMiddle, scratchMiddleRevision, damagedSuccessor); err != nil {
		return err
	}

	// One: the middle lineage's own edge, damaged by an edited reason.
	movedMiddleRevision, err := damageRecordedReason(sandbox, damagedMiddle, " (edited after the fact)")
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchMiddleRevision] = movedMiddleRevision

	// Two: the newest edge, whose recorded predecessor revision follows the
	// middle lineage while its authorization stays behind.
	successorRevision, err := damageAuthorizationPredecessorRevision(sandbox, damagedSuccessor, movedMiddleRevision)
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchSuccessorRevision] = successorRevision
	return requireInvalidEdges(sandbox, 2, theExactBindingProblem)
}

// danglingPredecessor removes the predecessor entry a valid successor names.
// Nothing is edited: this is the shape a partially restored backup, or a
// half-finished manual cleanup, leaves behind, and it exercises a different
// product path — the successor's edge is never classified at all, because there
// is nothing to classify it against.
func danglingPredecessor(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := mintSuccessor(sandbox, widenWithProse, scratchPredecessor, scratchPredecessorRevision, damagedSuccessor); err != nil {
		return err
	}
	predecessor, err := scratchValue(sandbox, scratchPredecessor)
	if err != nil {
		return err
	}
	directory, err := storeLineageDir(sandbox, predecessor)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(directory); err != nil {
		return err
	}
	return requireInvalidEdges(sandbox, 1, "missing predecessor")
}

// halfWrittenSuccessor leaves the successor's record truncated, which is what a
// process killed between opening the file and finishing the write leaves. The
// entry cannot be parsed, so it never becomes an edge at all: it lands in the
// inspection's entry diagnostics, which is a third distinct product path.
func halfWrittenSuccessor(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := mintSuccessor(sandbox, widenWithProse, scratchPredecessor, scratchPredecessorRevision, damagedSuccessor); err != nil {
		return err
	}
	if err := halveStateFile(sandbox, damagedSuccessor); err != nil {
		return err
	}
	inspection, err := proveInspection(sandbox)
	if err != nil {
		return err
	}
	if inspection.Complete {
		return errors.New("fixture claims an unreadable store entry but inspect-authority reports the inventory complete")
	}
	if inspection.Totals.EntryDiagnostics != 1 || len(inspection.EntryDiagnostics) != 1 {
		return fmt.Errorf("fixture claims exactly one unreadable entry but inspect-authority reports %+v", inspection.Totals)
	}
	diagnostic := inspection.EntryDiagnostics[0]
	if diagnostic.LineageID != damagedSuccessor || diagnostic.Problem != "malformed_compact_state" {
		return fmt.Errorf("fixture claims a malformed %q but the product reports %+v", damagedSuccessor, diagnostic)
	}
	if inspection.Totals.Edges != 0 {
		return fmt.Errorf("fixture claims the truncated entry never becomes an edge but inspect-authority reports %d", inspection.Totals.Edges)
	}
	return requireStoreNotAuthoritative(sandbox)
}

// ---------------------------------------------------------------------------
// Counted operator work
// ---------------------------------------------------------------------------

// reconcileArgs assembles the one operation whose entire job is quarantining a
// recovery edge that does not re-derive. The authorization is built by hand
// from values the product published, which is what the CLI help asks for — so
// the refusal, when it comes, is never a refusal of a malformed request.
func reconcileArgs(predecessorKey, predecessorRevisionKey, successorKey, successorRevisionKey, reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		predecessor, err := scratchValue(sandbox, predecessorKey)
		if err != nil {
			return nil, err
		}
		predecessorRevision, err := scratchValue(sandbox, predecessorRevisionKey)
		if err != nil {
			return nil, err
		}
		successor, err := scratchValue(sandbox, successorKey)
		if err != nil {
			return nil, err
		}
		successorRevision, err := scratchValue(sandbox, successorRevisionKey)
		if err != nil {
			return nil, err
		}
		const actor = "bench"
		authorization := strings.Join([]string{
			"gentle-ai.review-reconcile-authorization/v1",
			"predecessor_lineage=" + predecessor,
			"predecessor_revision=" + predecessorRevision,
			"successor_lineage=" + successor,
			"successor_revision=" + successorRevision,
			"actor=" + actor,
			"reason=" + reason,
		}, "\n")
		return []string{
			"review", "reconcile-authority", "--cwd", sandbox.Repo,
			"--predecessor-lineage", predecessor,
			"--expected-predecessor-revision", predecessorRevision,
			"--successor-lineage", successor,
			"--expected-successor-revision", successorRevision,
			"--actor", actor,
			"--reason", reason,
			"--maintainer-authorization", authorization,
		}, nil
	}
}

// abandonArgs assembles the exit that DOES clear a pristine damaged successor,
// and it costs the same six-line hand-built binding `review abandon` always
// costs — which is why every journey that reaches it also moves manual_tokens.
func abandonArgs(lineageKey, revisionKey, snapshotKey, reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		lineage, err := scratchValue(sandbox, lineageKey)
		if err != nil {
			return nil, err
		}
		revision, err := scratchValue(sandbox, revisionKey)
		if err != nil {
			return nil, err
		}
		snapshot, err := scratchValue(sandbox, snapshotKey)
		if err != nil {
			return nil, err
		}
		const actor = "bench"
		authorization := strings.Join([]string{
			"gentle-ai.review-abandon-authorization/v1",
			"lineage=" + lineage,
			"revision=" + revision,
			"snapshot_identity=" + snapshot,
			"actor=" + actor,
			"reason=" + reason,
		}, "\n")
		return []string{
			"review", "abandon", "--cwd", sandbox.Repo,
			"--lineage", lineage,
			"--expected-revision", revision,
			"--reason", reason,
			"--actor", actor,
			"--maintainer-authorization", authorization,
		}, nil
	}
}

// abandonLiveArgs is abandonArgs for a step that runs AFTER something moved the
// lineage's revision. It re-reads both bound values out of the product instead
// of using what the fixture published, so the refusal it measures is the
// refusal the journey is about and never a stale-revision refusal wearing its
// place. The read is uncounted because it is the benchmark keeping its own
// argv honest, not operator work: the operator reached this step by following
// the previous one.
func abandonLiveArgs(lineageKey, reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		lineage, err := scratchValue(sandbox, lineageKey)
		if err != nil {
			return nil, err
		}
		status, err := proveStoreStatus(sandbox)
		if err != nil {
			return nil, err
		}
		for _, entry := range status.Entries {
			if entry.LineageID != lineage {
				continue
			}
			if entry.Revision == "" || entry.SnapshotIdentity == "" {
				return nil, fmt.Errorf("review status lists %q with revision %q and snapshot %q, so the binding cannot be assembled",
					lineage, entry.Revision, entry.SnapshotIdentity)
			}
			sandbox.Scratch[scratchLiveRevision] = entry.Revision
			sandbox.Scratch[scratchLiveSnapshot] = entry.SnapshotIdentity
			return abandonArgs(lineageKey, scratchLiveRevision, scratchLiveSnapshot, reason)(sandbox)
		}
		return nil, fmt.Errorf("review status no longer lists %q", lineage)
	}
}

func reclaimArgs(lineageKey, reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		lineage, err := scratchValue(sandbox, lineageKey)
		if err != nil {
			return nil, err
		}
		return []string{
			"review", "reclaim", "--cwd", sandbox.Repo,
			"--lineage", lineage, "--actor", "bench", "--reason", reason,
		}, nil
	}
}

func invalidateArgs(lineageKey, revisionKey, reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		lineage, err := scratchValue(sandbox, lineageKey)
		if err != nil {
			return nil, err
		}
		revision, err := scratchValue(sandbox, revisionKey)
		if err != nil {
			return nil, err
		}
		return []string{
			"review", "invalidate", "--cwd", sandbox.Repo,
			"--lineage", lineage, "--expected-revision", revision,
			"--gate", "post-apply", "--reason", reason,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// Assertions attached to steps that do not block
// ---------------------------------------------------------------------------

// inspectionAssertion turns an expectation about `review inspect-authority`
// into an After hook, so a journey whose interesting answer is "the product
// still describes the damage correctly" fails loudly rather than passing
// quietly.
func inspectionAssertion(name string, check func(storeInspection) error) func(*Sandbox, Observation) error {
	return func(_ *Sandbox, observation Observation) error {
		var inspection storeInspection
		if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &inspection); err != nil {
			return fmt.Errorf("parse inspect-authority: %w (stderr: %s)", err, firstLine(observation.Stderr))
		}
		if err := check(inspection); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
}

func invalidEdgesWithNoAnomalyClass(edges int) func(storeInspection) error {
	return func(inspection storeInspection) error {
		if inspection.Totals.InvalidEdges != edges {
			return fmt.Errorf("invalid_edges = %d, want %d", inspection.Totals.InvalidEdges, edges)
		}
		for _, edge := range inspection.Edges {
			if len(edge.AnomalyClasses) != 0 {
				return fmt.Errorf("edge %q now publishes anomaly_classes %v, so it is reconcilable and this journey measures the wrong state",
					edge.SuccessorLineageID, edge.AnomalyClasses)
			}
		}
		return nil
	}
}

// proveStoreRecovered asserts an exit really put the store back in charge. An
// exit that runs, exits 0 and leaves the store non-authoritative is a worse
// answer than a refusal, and it is invisible from the exit code alone.
func proveStoreRecovered(r *journeyRun) error {
	status, err := proveStoreStatus(r.sandbox)
	if err != nil {
		return err
	}
	if !status.Authoritative || !status.Complete {
		return fmt.Errorf("the exit ran but review status still reports complete=%v authoritative=%v",
			status.Complete, status.Authoritative)
	}
	return nil
}

// proveStoreStillDamaged is its mirror, for the steps that claim an operation
// changed nothing.
func proveStoreStillDamaged(r *journeyRun) error {
	return requireStoreNotAuthoritative(r.sandbox)
}

// repairAssessment is the subset of `review repair --preflight` that says
// whether classified repair covers this damage.
type repairAssessment struct {
	Assessment struct {
		Status string `json:"status"`
		Counts struct {
			EligibleCandidates  int `json:"eligible_candidates"`
			UnsupportedLineages int `json:"unsupported_lineages"`
		} `json:"counts"`
	} `json:"assessment"`
}

// repairOffersNothing is the assertion attached to the preflight step. The
// preflight exits 0, so it is not a block and cannot be counted as one — but a
// journey that claims every advertised surface refused has to have asked this
// one too, and has to fail loudly on the day it starts answering.
func repairOffersNothing(_ *Sandbox, observation Observation) error {
	var assessment repairAssessment
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &assessment); err != nil {
		return fmt.Errorf("parse review repair --preflight: %w (stderr: %s)", err, firstLine(observation.Stderr))
	}
	if assessment.Assessment.Status != "unsupported" || assessment.Assessment.Counts.EligibleCandidates != 0 {
		return fmt.Errorf(
			"classified repair now reports status %q with %d eligible candidate(s); it covers this damage, so the journey's claim that every advertised surface refuses is stale",
			assessment.Assessment.Status, assessment.Assessment.Counts.EligibleCandidates)
	}
	if assessment.Assessment.Counts.UnsupportedLineages == 0 {
		return errors.New("classified repair reports no unsupported lineage, so it is not looking at the damaged one")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Wave 2 leaf authority disposition plan (Slice S4)
// ---------------------------------------------------------------------------
//
// review repair --preflight (Slice S3, internal/cli/review_repair.go) now
// surfaces a SECOND, unrelated surface beside the legacy classified-repair
// assessment above: disposition_provider_inputs{plan_digest,
// authority_inventory_revision} for a cardinality-one closed content-mismatch
// leaf (rdd-authority-disposition-plan). It deliberately never publishes the
// plan's repository binding or anomaly class — see apply-progress's S3
// Implementation Notes ("Building a valid --authorization requires
// reviewtransaction.DeriveAuthorityDispositionPlanAtRepo (Go API)") — so this
// axis, which already couples itself to the persisted store format (file doc
// comment above), extends that same coupling to the one product-internal
// value the CLI output never carries: the sha256 repository binding
// authorityRepairRoot (internal/reviewtransaction/authority_repair.go)
// derives from the resolved git common directory.
//
// plan_digest is directly reusable from --preflight output: the digest's
// pre-image excludes actor and reason (execution-time provenance, not plan
// identity — authority_disposition_plan.go), so --preflight's published
// plan_digest (always derived with actor="" reason="") is exactly the digest
// execution re-derives with the real --actor/--reason. This axis drives the
// documented two-step CLI flow for real: run --preflight, remember its
// plan_digest and authority_inventory_revision, then execute with exactly
// those values (fixed by the disposition-plan-digest pre-image narrowing;
// previously this axis had to re-derive plan_digest itself because the two
// values could never match — see git history for the prior workaround).

// dispositionRepairResult is the subset of review repair's JSON envelope
// Slice S3 added for the plan-bound leaf authority disposition surface
// (internal/cli/review_repair.go ReviewRepairResult): the plan-bound
// preflight preview and the committed execution's safe, path-free
// projection.
type dispositionRepairResult struct {
	DispositionProviderInputs *struct {
		PlanDigest                 string `json:"plan_digest"`
		AuthorityInventoryRevision string `json:"authority_inventory_revision"`
	} `json:"disposition_provider_inputs"`
	DispositionExecution *struct {
		Status                     string `json:"status"`
		LineageID                  string `json:"lineage_id"`
		PlanDigest                 string `json:"plan_digest"`
		AuthorityInventoryRevision string `json:"authority_inventory_revision"`
	} `json:"disposition_execution"`
}

// authorityDispositionPlanSchema and authorityDispositionAuthorizationSchema
// mirror reviewtransaction.AuthorityDispositionPlanSchema and the exported
// disposition authorization schema constant
// (internal/reviewtransaction/authority_disposition_plan.go).
// contentMismatchedRecoveryAuthorizationClass mirrors that package's
// unexported compactContentMismatchedRecoveryAuthorizationClass value — the
// one closed anomaly class Wave 2 derives a plan for.
const (
	authorityDispositionPlanSchema              = "gentle-ai.review-authority-disposition-plan/v1"
	authorityDispositionAuthorizationSchema     = "gentle-ai.review-disposition-authorization/v1"
	contentMismatchedRecoveryAuthorizationClass = "content_mismatched_recovery_authorization"
	dispositionRepositoryBindingDomain          = "gentle-ai.review-repository-binding/v1\n"
	dispositionWitnessLineage                   = "review-damaged-disposition-witness"
	scratchDispositionPlanDigest                = "damaged-store/disposition-plan-digest"
	scratchDispositionInventoryRevision         = "damaged-store/disposition-inventory-revision"
	scratchDispositionWitnessLineage            = "damaged-store/disposition-witness-lineage"
	scratchDispositionWitnessBytes              = "damaged-store/disposition-witness-bytes"
)

// validDispositionSHA256 is this axis's own check for the shape
// review_repair.go's validReviewCapabilitySHA256 enforces on the product
// side — this axis is a separate Go module and cannot import internal/cli.
func validDispositionSHA256(value string) bool {
	const prefix = "sha256:"
	return strings.HasPrefix(value, prefix) && len(value) == len(prefix)+64
}

// dispositionRepositoryBinding re-derives the sha256 repository binding
// authorityRepairRoot computes over the resolved git common directory. It
// mirrors deriveRevision above: a value this axis reproduces independently
// and proves correct only by the product accepting the resulting
// authorization, never assumed.
func dispositionRepositoryBinding(sandbox *Sandbox) (string, error) {
	common, err := gitCommonDir(sandbox, sandbox.Repo)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(dispositionRepositoryBindingDomain + common))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// dispositionAuthorization renders the exact seven-line
// gentle-ai.review-disposition-authorization/v1 binding
// authorityDispositionAuthorizationBinding
// (authority_disposition_plan.go) computes internally, mirroring how
// reconcileArgs and abandonArgs above hand-render their own authorization
// bindings rather than depending on an exported production helper.
func dispositionAuthorization(binding, planDigest, inventoryRevision, actor, reason string) string {
	return strings.Join([]string{
		authorityDispositionAuthorizationSchema,
		"schema=" + authorityDispositionPlanSchema,
		"repository=" + binding,
		"class=" + contentMismatchedRecoveryAuthorizationClass,
		"plan_digest=" + planDigest,
		"inventory_revision=" + inventoryRevision,
		"actor=" + actor,
		"reason=" + reason,
	}, "\n")
}

// approvedUnrelatedDispositionWitness builds one approved review with no
// causal relationship to the recovery graph the disposition journeys damage,
// and records the exact bytes its store entry holds. The retained-graph step
// later requires those bytes unchanged — proof that repairing one leaf's
// authority disposition never touches the graph elsewhere (design Testing
// Strategy: "Retained graph").
func approvedUnrelatedDispositionWitness(sandbox *Sandbox) error {
	if err := sandbox.write(filepath.Join(sandbox.Repo, "docs", "disposition-witness.md"), "# witness\n\nunrelated documentation.\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "-A"); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "start", "--cwd", sandbox.Repo, "--lineage", dispositionWitnessLineage); err != nil {
		return err
	}
	if _, err := fixtureCommand(sandbox, "review", "finalize", "--cwd", sandbox.Repo, "--lineage", dispositionWitnessLineage); err != nil {
		return err
	}
	path, err := storeStatePath(sandbox, dispositionWitnessLineage)
	if err != nil {
		return err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchDispositionWitnessLineage] = dispositionWitnessLineage
	sandbox.Scratch[scratchDispositionWitnessBytes] = string(payload)
	return nil
}

// damagedLeafEligibleForDisposition is damagedEdgeWithResults's exact shape —
// a non-pristine successor (it captured a reviewer result, so review abandon
// refuses it) over a content-mismatched recovery edge — plus one unrelated
// approved witness lineage the repair journey below proves untouched.
func damagedLeafEligibleForDisposition(sandbox *Sandbox) error {
	if err := approvedPredecessor(sandbox); err != nil {
		return err
	}
	if err := approvedUnrelatedDispositionWitness(sandbox); err != nil {
		return err
	}
	if err := mintSuccessor(sandbox, widenWithCode, scratchPredecessor, scratchPredecessorRevision, damagedSuccessor); err != nil {
		return err
	}
	if err := captureOneLensResult(sandbox); err != nil {
		return err
	}
	revision, err := damageRecordedReason(sandbox, damagedSuccessor, " (edited after the fact)")
	if err != nil {
		return err
	}
	sandbox.Scratch[scratchSuccessorRevision] = revision
	return requireInvalidEdges(sandbox, 1, theExactBindingProblem)
}

// requireDispositionPlanEligible is the After hook for the plan-bound
// preflight step: proves review repair --preflight surfaced a disposition
// plan for exactly this content-mismatched leaf, and remembers the plan
// digest and inventory revision the execution step then binds.
func requireDispositionPlanEligible(sandbox *Sandbox, observation Observation) error {
	var result dispositionRepairResult
	if err := decodeWaveObservation(observation, &result, "review repair --preflight disposition plan"); err != nil {
		return err
	}
	if result.DispositionProviderInputs == nil ||
		!validDispositionSHA256(result.DispositionProviderInputs.PlanDigest) ||
		!validDispositionSHA256(result.DispositionProviderInputs.AuthorityInventoryRevision) {
		return fmt.Errorf("review repair --preflight did not surface an eligible leaf authority disposition plan: %+v", result)
	}
	sandbox.Scratch[scratchDispositionPlanDigest] = result.DispositionProviderInputs.PlanDigest
	sandbox.Scratch[scratchDispositionInventoryRevision] = result.DispositionProviderInputs.AuthorityInventoryRevision
	return nil
}

// requireNoDispositionPlanSurfaced is the mirror assertion for a shape that
// must NOT admit a disposition plan: more than one closed content-mismatch
// edge (design decision 5, "cardinality is executor policy" — derivation
// itself refuses ambiguity before admission is ever asked).
func requireNoDispositionPlanSurfaced(_ *Sandbox, observation Observation) error {
	var result dispositionRepairResult
	if err := decodeWaveObservation(observation, &result, "review repair --preflight multi-edge shape"); err != nil {
		return err
	}
	if result.DispositionProviderInputs != nil {
		return fmt.Errorf("review repair --preflight surfaced a disposition plan for an ambiguous multi-edge shape: %+v", result)
	}
	return nil
}

// dispositionRepairExecutionInputs gathers everything both
// dispositionRepairArgs and forgedDispositionRepairArgs need: the plan_digest
// and authority_inventory_revision --preflight published (both reusable
// as-is: plan_digest's pre-image excludes actor/reason, so the value
// --preflight surfaces is exactly the digest execution validates against),
// plus the independently re-derived repository binding needed to render the
// authorization text (--preflight never publishes it).
func dispositionRepairExecutionInputs(sandbox *Sandbox) (planDigest, inventoryRevision, binding string, err error) {
	if planDigest, err = scratchValue(sandbox, scratchDispositionPlanDigest); err != nil {
		return
	}
	if inventoryRevision, err = scratchValue(sandbox, scratchDispositionInventoryRevision); err != nil {
		return
	}
	binding, err = dispositionRepositoryBinding(sandbox)
	return
}

// dispositionRepairArgs assembles the leaf authority disposition execution
// review repair asks for: --plan-digest and --inventory-revision copied
// directly from requireDispositionPlanEligible's --preflight scratch values,
// plus an authorization this axis renders by hand (the repository binding
// and anomaly class --preflight never publishes).
func dispositionRepairArgs(reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		planDigest, inventoryRevision, binding, err := dispositionRepairExecutionInputs(sandbox)
		if err != nil {
			return nil, err
		}
		const actor = "bench"
		authorization := dispositionAuthorization(binding, planDigest, inventoryRevision, actor, reason)
		return []string{
			"review", "repair", "--cwd", sandbox.Repo,
			"--plan-digest", planDigest, "--inventory-revision", inventoryRevision,
			"--actor", actor, "--reason", reason, "--authorization", authorization,
		}, nil
	}
}

// forgedDispositionRepairArgs is dispositionRepairArgs with the CORRECT
// plan_digest and inventory_revision (so CAS and plan-match both pass) but
// an authorization bound to a repository identity that can never be the real
// one — isolating mandatory obligation (b) (tasks.md 2.4): a forged
// authorization refuses even when the plan digest and inventory revision
// both match exactly.
func forgedDispositionRepairArgs(reason string) func(*Sandbox) ([]string, error) {
	return func(sandbox *Sandbox) ([]string, error) {
		planDigest, inventoryRevision, _, err := dispositionRepairExecutionInputs(sandbox)
		if err != nil {
			return nil, err
		}
		const actor = "bench"
		forgedBinding := "sha256:" + strings.Repeat("f", 64)
		forgedAuthorization := dispositionAuthorization(forgedBinding, planDigest, inventoryRevision, actor, reason)
		return []string{
			"review", "repair", "--cwd", sandbox.Repo,
			"--plan-digest", planDigest, "--inventory-revision", inventoryRevision,
			"--actor", actor, "--reason", reason, "--authorization", forgedAuthorization,
		}, nil
	}
}

// requireDispositionQuarantineCommitted is the After hook for the disposition
// execution step: the committed quarantine proof binds the exact plan digest
// --preflight published, and the response never repeats the repository path
// or the authorization text it was given.
func requireDispositionQuarantineCommitted(sandbox *Sandbox, observation Observation) error {
	var result dispositionRepairResult
	if err := decodeWaveObservation(observation, &result, "review repair leaf authority disposition execution"); err != nil {
		return err
	}
	planDigest, err := scratchValue(sandbox, scratchDispositionPlanDigest)
	if err != nil {
		return err
	}
	if result.DispositionExecution == nil || result.DispositionExecution.Status != "committed" ||
		result.DispositionExecution.PlanDigest != planDigest {
		return fmt.Errorf("leaf authority disposition execution did not commit the expected plan: %+v", result)
	}
	if strings.Contains(observation.Stdout, sandbox.Repo) {
		return errors.New("leaf authority disposition execution leaked the repository path")
	}
	return nil
}

// requireRetainedGraphValid is the post-repair inspection assertion (design
// Testing Strategy: "Retained graph") — the quarantined leaf's own edge is
// gone and what remains re-derives cleanly.
func requireRetainedGraphValid(inspection storeInspection) error {
	if !inspection.Complete || !inspection.Valid {
		return fmt.Errorf("post-repair inspection is not complete and valid: complete=%v valid=%v", inspection.Complete, inspection.Valid)
	}
	if inspection.Totals.Edges != 0 || inspection.Totals.InvalidEdges != 0 {
		return fmt.Errorf("post-repair inspection still reports a recovery edge: %+v", inspection.Totals)
	}
	return nil
}

// requireDispositionWitnessBytesUnchanged is the retained-graph proof this
// axis can make that an integration-level test cannot as directly: the
// unrelated witness lineage's own store entry holds the exact bytes it held
// before the damaged leaf was ever repaired.
func requireDispositionWitnessBytesUnchanged(r *journeyRun) error {
	lineage, err := scratchValue(r.sandbox, scratchDispositionWitnessLineage)
	if err != nil {
		return err
	}
	before, err := scratchValue(r.sandbox, scratchDispositionWitnessBytes)
	if err != nil {
		return err
	}
	path, err := storeStatePath(r.sandbox, lineage)
	if err != nil {
		return err
	}
	after, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(after) != before {
		return errors.New("repairing the damaged leaf changed the unrelated witness lineage's own store bytes")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

var inspectAuthorityCapability = &Capability{
	Verb:  []string{"review", "inspect-authority"},
	Flags: []string{"--cwd"},
}

var reconcileAuthorityCapability = &Capability{
	Verb: []string{"review", "reconcile-authority"},
	Flags: []string{"--cwd", "--predecessor-lineage", "--expected-predecessor-revision",
		"--successor-lineage", "--expected-successor-revision", "--maintainer-authorization"},
}

var reclaimAuthorityCapability = &Capability{
	Verb:  []string{"review", "reclaim"},
	Flags: []string{"--cwd", "--lineage", "--actor", "--reason"},
}

var invalidateReasonCapability = &Capability{
	Verb:  []string{"review", "invalidate"},
	Flags: []string{"--cwd", "--lineage", "--expected-revision", "--gate", "--reason"},
}

var abandonAxisCapability = &Capability{
	Verb:  []string{"review", "abandon"},
	Flags: []string{"--cwd", "--lineage", "--expected-revision", "--reason", "--actor", "--maintainer-authorization"},
}

var repairPreflightCapability = &Capability{
	Verb:  []string{"review", "repair"},
	Flags: []string{"--cwd", "--preflight"},
}

var repairDispositionExecuteCapability = &Capability{
	Verb:  []string{"review", "repair"},
	Flags: []string{"--cwd", "--plan-digest", "--inventory-revision", "--actor", "--reason", "--authorization"},
}

// ---------------------------------------------------------------------------
// Corpus
// ---------------------------------------------------------------------------

func damagedStoreJourneys() []Journey {
	return []Journey{
		{
			ID:     "ds01-two-recovery-edges-neither-admitted",
			Title:  "The reported shape: two recovery edges, both correctly prefixed, neither admitted by anything",
			Source: "community report (damaged compact-v2 store) + shape 4",
			// This is the state the report describes and the triage
			// reproduced. Both edges carry an authorization with the right
			// schema binding content the record no longer holds; the product
			// publishes `anomaly_classes: []` for both, which is how it says
			// the edges are outside the two classes reconciliation knows.
			//
			// Expected: the operator can SEE the damage — inspect-authority
			// describes both edges precisely, which is the product at its best
			// — and then every advertised surface refuses. The gate refuses,
			// `review start` refuses, the one operation whose entire job is
			// quarantining an edge that does not re-derive refuses on both
			// edges, reclaim refuses, classified repair reports it does not
			// cover this, and the abandonment that clears the single-edge
			// shape in ds02 refuses here before it even reaches the successor:
			// it will not leave the remaining graph invalid.
			//
			// So this journey declares `dead_end`, and the declaration is
			// carried by its own steps rather than by an opinion. Whether any
			// refusal on the way names something runnable is the measurement,
			// and rule 3 of the classifier outranks the declaration the moment
			// one does.
			Steps: []Step{
				{Name: "fixture: two damaged recovery edges", Fixture: damagedEdgePair},
				{Name: "inspect the authority, which is what an operator does first",
					Requires: inspectAuthorityCapability,
					Args:     productArgs("review", "inspect-authority"),
					After: inspectionAssertion("the reported shape at measurement time",
						invalidEdgesWithNoAnomalyClass(2))},
				{Name: "the delivery gate over a damaged store", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "post-apply")},
				{Name: "start a fresh review instead", Requires: startCapability,
					Args: productArgs("review", "start")},
				{Name: "reconcile the newest edge, which is the operation for exactly this",
					Requires: reconcileAuthorityCapability,
					Args: reconcileArgs(scratchMiddle, scratchMiddleRevision, scratchSuccessor, scratchSuccessorRevision,
						"the recovery edge does not re-derive")},
				{Name: "reconcile the older edge", Requires: reconcileAuthorityCapability,
					Args: reconcileArgs(scratchPredecessor, scratchPredecessorRevision, scratchMiddle, scratchMiddleRevision,
						"the recovery edge does not re-derive")},
				{Name: "reclaim the newest successor entry", Requires: reclaimAuthorityCapability,
					Args: reclaimArgs(scratchSuccessor, "the recovery edge does not re-derive")},
				{Name: "ask classified repair whether it covers this", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: repairOffersNothing},
				// The abandonment that clears the SINGLE-edge shape in ds02 is
				// the last thing left, and here it refuses before it looks at
				// the successor at all: quarantining one of two damaged edges
				// would leave the graph invalid, so it will not act. That is a
				// correct guard, and it is the one that closes the last door.
				//
				// Everything above has now been driven and answered, which is
				// what the declaration on this step rests on.
				{Name: "abandon the newest successor, which nothing named", Requires: abandonAxisCapability,
					Args: abandonArgs(scratchSuccessor, scratchSuccessorRevision, scratchSuccessorSnapshot,
						"the recovery edge cannot be admitted"),
					DeadEnd: true},
				{Name: "the store is still not in charge", Composite: proveStoreStillDamaged},
			},
		},
		{
			ID:     "ds02-damaged-edge-pristine-successor",
			Title:  "One damaged edge over a pristine successor: an exit exists and nothing names it",
			Source: "community report + triage finding (abandon clears it) + shape 4",
			// The triage's finding was that this shape is recoverable. The
			// successor never captured anything, so `review abandon` accepts
			// it, quarantines it, and the approved predecessor is back in
			// charge.
			//
			// Expected: the gate and the reconciliation refuse, neither naming
			// the abandonment; the abandonment then works. The defect this
			// journey measures is not that the operator is stuck — it is that
			// they cannot get out by running only what the messages named, and
			// the last step proves the exit was real by requiring the store to
			// govern again.
			Steps: []Step{
				{Name: "fixture: one damaged recovery edge, pristine successor", Fixture: damagedEdgePristine},
				{Name: "inspect the authority", Requires: inspectAuthorityCapability,
					Args: productArgs("review", "inspect-authority"),
					After: inspectionAssertion("one edge outside every anomaly class",
						invalidEdgesWithNoAnomalyClass(1))},
				{Name: "the delivery gate over a damaged store", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "post-apply")},
				{Name: "reconcile the edge, which is the operation for exactly this",
					Requires: reconcileAuthorityCapability,
					Args: reconcileArgs(scratchPredecessor, scratchPredecessorRevision, scratchSuccessor, scratchSuccessorRevision,
						"the recovery edge does not re-derive")},
				{Name: "abandon the successor, which nothing named", Requires: abandonAxisCapability,
					Args: abandonArgs(scratchSuccessor, scratchSuccessorRevision, scratchSuccessorSnapshot,
						"the recovery edge cannot be admitted")},
				{Name: "the store governs again", Composite: proveStoreRecovered},
			},
		},
		{
			ID:     "ds03-damaged-edge-successor-holds-results",
			Title:  "The same edge under a successor that captured a lens result: the pristineness rule refuses",
			Source: "community report + triage finding (abandon refuses non-pristine) + shape 3",
			// Same damage, one difference: the successor holds a reviewer
			// result the product itself wrote. `review abandon` is right to
			// refuse — abandoning a lineage that holds captured review work
			// would discard it — and reconciliation is right to refuse an edge
			// outside its two classes. Both guards are correct and together
			// they leave nothing.
			//
			// This is the one journey in the axis that declares `dead_end`, and
			// the declaration is the most expensive claim this benchmark
			// allows: it says no continuation exists at all. It is not
			// asserted from reading the product — it is evidenced by the steps
			// in front of it, which drive every advertised authority-repair
			// surface in turn and record what each one answered:
			//
			//   reconcile-authority  refuses: the edge is outside its classes
			//   reclaim              refuses: the entry holds authority
			//   repair --preflight   exits 0 reporting `unsupported`
			//   invalidate           exits 0 and changes nothing
			//   abandon              refuses: the successor holds a result
			//
			// Every one of those refusals is CORRECT in isolation. That is
			// what makes the shape worth measuring: correct guards composing
			// into no way out is not visible from any one of them.
			//
			// That composition is now closed. `review abandon` names the same
			// exit `review reclaim` already named — capture the machine-readable
			// diagnosis with `review inspect-authority` and escalate it — so the
			// last refusal in the chain leaves the operator somewhere and this
			// journey no longer declares a dead end. The steps stay exactly as
			// they were: they still drive every advertised repair surface, and
			// they are what would catch the exit going away again.
			Steps: []Step{
				{Name: "fixture: one damaged recovery edge, successor holds a captured result", Fixture: damagedEdgeWithResults},
				{Name: "inspect the authority", Requires: inspectAuthorityCapability,
					Args: productArgs("review", "inspect-authority"),
					After: inspectionAssertion("one edge outside every anomaly class",
						invalidEdgesWithNoAnomalyClass(1))},
				{Name: "reconcile the edge", Requires: reconcileAuthorityCapability,
					Args: reconcileArgs(scratchPredecessor, scratchPredecessorRevision, scratchSuccessor, scratchSuccessorRevision,
						"the recovery edge does not re-derive")},
				{Name: "reclaim the entry instead", Requires: reclaimAuthorityCapability,
					Args: reclaimArgs(scratchSuccessor, "the recovery edge cannot be admitted")},
				{Name: "ask classified repair whether it covers this", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: repairOffersNothing},
				{Name: "invalidate the successor", Requires: invalidateReasonCapability,
					Args: invalidateArgs(scratchSuccessor, scratchSuccessorRevision, "the recovery edge cannot be admitted")},
				{Name: "the invalidation ran and changed nothing about the damage", Composite: proveStoreStillDamaged},
				{Name: "abandon the successor, which cleared the pristine one", Requires: abandonAxisCapability,
					Args: abandonLiveArgs(scratchSuccessor, "the recovery edge cannot be admitted")},
				{Name: "the store is still not in charge", Composite: proveStoreStillDamaged},
			},
		},
		{
			ID:     "ds04-recovery-edge-with-no-predecessor",
			Title:  "A successor whose predecessor entry is gone: the edge is never classified at all",
			Source: "damaged store by partial restore + shape 4",
			// A different product path from the three above: the successor's
			// recovery names a lineage the store no longer holds, so the edge
			// is never handed to the classifier — it is reported as a missing
			// predecessor with no anomaly class, and `review start` refuses
			// with a dangling-predecessor message rather than a binding one.
			//
			// The successor here is pristine, so the same abandonment that
			// worked in ds02 should work, and the journey ends by proving the
			// store governs again. What is measured is whether anything on the
			// way named it.
			Steps: []Step{
				{Name: "fixture: a successor whose predecessor entry is gone", Fixture: danglingPredecessor},
				{Name: "inspect the authority", Requires: inspectAuthorityCapability,
					Args: productArgs("review", "inspect-authority"),
					After: inspectionAssertion("a missing predecessor is not an anomaly class",
						invalidEdgesWithNoAnomalyClass(1))},
				{Name: "start a fresh review over the damaged graph", Requires: startCapability,
					Args: productArgs("review", "start")},
				{Name: "abandon the orphaned successor", Requires: abandonAxisCapability,
					Args: abandonArgs(scratchSuccessor, scratchSuccessorRevision, scratchSuccessorSnapshot,
						"its predecessor is gone")},
				{Name: "the store governs again", Composite: proveStoreRecovered},
			},
		},
		{
			ID:     "ds05-half-written-successor-record",
			Title:  "A record truncated mid-write: the refusal names a continuation that cannot load it",
			Source: "interrupted write + shape 4",
			// The third distinct path: the entry never parses, so it never
			// becomes an edge. It lands in the inspection's entry diagnostics
			// as `malformed_compact_state`, and `review reclaim` — the
			// operation for an incomplete entry — refuses it and names
			// `review reconcile-authority` as the operation that handles it
			// instead.
			//
			// Expected: that named continuation does not work here. It cannot
			// load the successor either, because loading it is the thing that
			// fails. This is shape 4 in its purest form and it is the reason
			// this journey is in the axis at all.
			Steps: []Step{
				{Name: "fixture: a successor record truncated mid-write", Fixture: halfWrittenSuccessor},
				{Name: "inspect the authority", Requires: inspectAuthorityCapability,
					Args: productArgs("review", "inspect-authority"),
					After: inspectionAssertion("one unreadable entry, and no edge at all", func(inspection storeInspection) error {
						if inspection.Complete {
							return errors.New("complete = true, so the unreadable entry is no longer reported")
						}
						if inspection.Totals.EntryDiagnostics != 1 || inspection.Totals.Edges != 0 {
							return fmt.Errorf("entry_diagnostics = %d and edges = %d, want 1 and 0",
								inspection.Totals.EntryDiagnostics, inspection.Totals.Edges)
						}
						return nil
					})},
				{Name: "the delivery gate over an unreadable entry", Requires: validateCapability,
					Args: productArgs("review", "validate", "--gate", "post-apply")},
				{Name: "reclaim the incomplete entry", Requires: reclaimAuthorityCapability,
					Args: reclaimArgs(scratchSuccessor, "the record is half written")},
				{Name: "reconcile it, which is what the reclaim refusal named",
					Requires: reconcileAuthorityCapability,
					Args: reconcileArgs(scratchPredecessor, scratchPredecessorRevision, scratchSuccessor, scratchSuccessorRevision,
						"the record is half written")},
				{Name: "the store is exactly as damaged as it was", Composite: proveStoreStillDamaged},
			},
		},
		{
			ID:     "ds06-content-mismatched-leaf-repaired-via-disposition-plan",
			Title:  "A non-pristine content-mismatched leaf: the leaf authority disposition plan repairs it black-box, and the rest of the graph never moves",
			Source: "rdd-root-simplification-wave2 Slice S2/S3 + community report shape 3",
			// ds03 above proves the guards around this exact shape are each
			// individually correct — reconcile refuses (outside its two
			// classes), reclaim refuses (the entry holds authority),
			// classified repair reports unsupported, invalidate changes
			// nothing, and abandon refuses because the successor holds a
			// captured reviewer result.
			//
			// What ds03 could not prove, because it predates this wave, is
			// Wave 2's own answer to that closed door: a leaf authority
			// disposition plan for this exact closed class.
			// `review repair --preflight` now surfaces a plan-bound digest
			// and inventory revision for it, and executing that plan
			// quarantines the leaf the same way abandon quarantines a
			// pristine one in ds02 — byte-preserving, with forensic residue,
			// and leaving the rest of the authority graph untouched. This
			// journey proves both halves: the repair actually lands, and an
			// unrelated lineage's own store bytes never move.
			Steps: []Step{
				{Name: "fixture: one damaged recovery edge, non-pristine successor, plus an unrelated witness lineage",
					Fixture: damagedLeafEligibleForDisposition},
				{Name: "inspect the authority, which is what an operator does first",
					Requires: inspectAuthorityCapability,
					Args:     productArgs("review", "inspect-authority"),
					After: inspectionAssertion("one edge outside every anomaly class",
						invalidEdgesWithNoAnomalyClass(1))},
				{Name: "ask what review repair would do", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: requireDispositionPlanEligible},
				{Name: "repair the leaf through its disposition plan", Requires: repairDispositionExecuteCapability,
					Args: dispositionRepairArgs("quarantine the content-mismatched leaf"), After: requireDispositionQuarantineCommitted},
				{Name: "the authority graph after repair", Requires: inspectAuthorityCapability,
					Args:  productArgs("review", "inspect-authority"),
					After: inspectionAssertion("the retained graph after repair", requireRetainedGraphValid)},
				{Name: "the store governs again", Composite: proveStoreRecovered},
				{Name: "the unrelated witness lineage never moved", Composite: requireDispositionWitnessBytesUnchanged},
			},
		},
		{
			ID:     "ds07-two-content-mismatched-edges-no-disposition-plan",
			Title:  "Two closed content-mismatch edges in one chain: the disposition plan refuses to pick one, and nothing is quarantined",
			Source: "rdd-root-simplification-wave2 design decision 5/6 (cardinality is executor policy) + shape 4",
			// ds01 above drives every legacy-era recovery surface over this
			// exact two-edge shape and finds none of them admit it. Wave 2
			// adds one more surface to check: whether the new leaf
			// authority disposition plan picks a side. It does not —
			// derivation itself refuses the moment it finds more than one
			// closed-class edge, so `review repair --preflight` never
			// surfaces a plan for either edge, and nothing about the store
			// changes for having asked.
			Steps: []Step{
				{Name: "fixture: two damaged recovery edges", Fixture: damagedEdgePair},
				{Name: "inspect the authority", Requires: inspectAuthorityCapability,
					Args: productArgs("review", "inspect-authority"),
					After: inspectionAssertion("the reported shape at measurement time",
						invalidEdgesWithNoAnomalyClass(2))},
				{Name: "ask what review repair would do", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: requireNoDispositionPlanSurfaced},
				{Name: "the store is still not in charge", Composite: proveStoreStillDamaged},
			},
		},
		{
			ID:     "ds08-content-mismatched-leaf-forged-authorization-refuses",
			Title:  "The same eligible leaf, an authorization bound to the wrong repository: execution refuses and quarantines nothing",
			Source: "rdd-root-simplification-wave2 tasks.md 2.4 (mandatory obligation (b)) + shape 3",
			// The same single-edge, non-pristine shape ds06 above repairs
			// cleanly, but here the authorization presented at execution
			// binds a repository identity that can never be the real one.
			// The executor re-derives the plan and its own repository
			// binding, finds them disagree with what was supplied, and
			// refuses before anything is quarantined — proving mandatory
			// obligation (b) (tasks.md 2.4) holds even when the plan digest
			// and inventory revision both match exactly.
			Steps: []Step{
				{Name: "fixture: one damaged recovery edge, non-pristine successor",
					Fixture: damagedEdgeWithResults},
				{Name: "inspect the authority", Requires: inspectAuthorityCapability,
					Args: productArgs("review", "inspect-authority"),
					After: inspectionAssertion("one edge outside every anomaly class",
						invalidEdgesWithNoAnomalyClass(1))},
				{Name: "ask what review repair would do", Requires: repairPreflightCapability,
					Args: productArgs("review", "repair", "--preflight=true"), After: requireDispositionPlanEligible},
				{Name: "repair with an authorization bound to the wrong repository", Requires: repairDispositionExecuteCapability,
					Args: forgedDispositionRepairArgs("quarantine the content-mismatched leaf")},
				{Name: "the store is still not in charge", Composite: proveStoreStillDamaged},
			},
		},
	}
}
