package managedstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/filecoord"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// Typed read-only refusals. They precede every write.
var (
	ErrUnsupportedNewerSchema = errors.New("managedstate: unsupported newer schema; read-only, zero writes")
	ErrUnknownAuthority       = errors.New("managedstate: invalid schema identity; read-only, zero writes")
	ErrStaleManifest          = errors.New("managedstate: manifest generation changed; stale CAS, no write")
	ErrInjected               = errors.New("managedstate: injected fault")
)

var errManifestWriteVerify = errors.New("managedstate: published manifest failed digest verification")

const (
	manifestSchemaPrefix = "gentle-ai.managed-assets/v"
	journalSchemaPrefix  = "gentle-ai.migration-journal/v"
	supportedVersion     = 1
)

// PlannedResource is one full-file resource a caller wants synced.
type PlannedResource struct {
	ID      string
	Adapter string
	Target  string // absolute path
	Desired []byte
	Mode    fs.FileMode
	Extent  state.OwnedExtent
}

// Plan is one migration transaction's intent.
type Plan struct {
	Producer  state.Producer
	Resources []PlannedResource
}

// Result reports what one Run did. Writes count mutations this run only.
type Result struct {
	TransactionID  string
	NoOp           bool
	Resumed        bool
	Phase          Phase
	Status         Status
	TargetWrites   int
	ManifestWrites int
}

// Faults are deterministic injection points at every durable boundary. A
// non-nil return aborts the run like a crash: durable state stays exactly as
// last persisted and no rollback runs. Faults are test-only seams; zero value
// is inert.
type Faults struct {
	BeforeJournal         func(Phase) error
	AfterJournal          func(Phase) error
	BeforeTargetWrite     func(target string) error
	AfterTargetWrite      func(target string) error
	BeforeManifestPublish func() error
	AfterManifestPublish  func() error
	Verify                func(target string) error
}

func (f Faults) beforeJournal(p Phase) error {
	if f.BeforeJournal == nil {
		return nil
	}
	return f.BeforeJournal(p)
}

func (f Faults) afterJournal(p Phase) error {
	if f.AfterJournal == nil {
		return nil
	}
	return f.AfterJournal(p)
}

func (f Faults) beforeTargetWrite(target string) error {
	if f.BeforeTargetWrite == nil {
		return nil
	}
	return f.BeforeTargetWrite(target)
}

func (f Faults) afterTargetWrite(target string) error {
	if f.AfterTargetWrite == nil {
		return nil
	}
	return f.AfterTargetWrite(target)
}

func (f Faults) beforeManifestPublish() error {
	if f.BeforeManifestPublish == nil {
		return nil
	}
	return f.BeforeManifestPublish()
}

func (f Faults) afterManifestPublish() error {
	if f.AfterManifestPublish == nil {
		return nil
	}
	return f.AfterManifestPublish()
}

func (f Faults) verify(target string) error {
	if f.Verify == nil {
		return nil
	}
	return f.Verify(target)
}

// Run executes or resumes one migration transaction under the exclusive
// managed-state lock, implementing the #1876 restart decision table.
func Run(ctx context.Context, homeDir string, plan Plan, faults Faults) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validatePlan(plan); err != nil {
		return nil, err
	}

	// The lock file is coordination state, never authority (design: locking
	// model). It is held from preflight through manifest publication.
	if err := os.MkdirAll(lockRoot(homeDir), 0o755); err != nil {
		return nil, err
	}
	lease, err := filecoord.Acquire(ctx, lockTarget(homeDir), lockRoot(homeDir))
	if err != nil {
		return nil, err // typed *filecoord.BusyError on contention
	}
	defer lease.Release()

	current, expectedDigest, err := readManifestGated(homeDir)
	if err != nil {
		return nil, err
	}

	j, err := LoadActiveJournal(homeDir)
	if err != nil && !errors.Is(err, ErrNoActiveJournal) {
		return nil, err
	}
	if j != nil {
		if err := gateJournalSchema(j.Schema); err != nil {
			return nil, err
		}
		if j.Status != "" {
			// Terminal journals are read-only truth: report, never rewrite.
			return &Result{TransactionID: j.TransactionID, Resumed: true, Phase: j.LastPhase, Status: j.Status}, nil
		}
		if phaseOrder[j.LastPhase] >= phaseOrder[PhasePrepared] {
			return resumeRun(ctx, homeDir, plan, j, faults)
		}
		// Pre-prepared leftovers mutated nothing; start clean.
		if err := os.RemoveAll(transactionDir(homeDir, j.TransactionID)); err != nil {
			return nil, err
		}
	}
	return freshRun(ctx, homeDir, plan, current, expectedDigest, faults)
}

func validatePlan(plan Plan) error {
	if len(plan.Resources) == 0 {
		return fmt.Errorf("managedstate: empty plan")
	}
	for _, r := range plan.Resources {
		if r.ID == "" || r.Target == "" || r.Desired == nil {
			return fmt.Errorf("managedstate: resource %q incomplete", r.ID)
		}
		if !filepath.IsAbs(r.Target) {
			return fmt.Errorf("managedstate: target %q must be absolute", r.Target)
		}
		if r.Extent.Kind != state.ExtentFull || r.Extent.Ownership != state.OwnershipManaged {
			return fmt.Errorf("managedstate: slice 1 supports managed full-file extents only (%s)", r.ID)
		}
	}
	return nil
}

func lockTarget(homeDir string) string { return filepath.Join(homeDir, state.StateDirName) }

func lockRoot(homeDir string) string { return filepath.Join(homeDir, state.StateDirName, "locks") }

// gateJournalSchema refuses journals this binary cannot speak for.
func gateJournalSchema(schema string) error {
	v, ok := parseVersion(journalSchemaPrefix, schema)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownAuthority, schema)
	}
	if v > supportedVersion {
		return fmt.Errorf("%w: %q", ErrUnsupportedNewerSchema, schema)
	}
	return nil
}

// readManifestGated returns the on-disk manifest and its bundle digest, or a
// typed read-only refusal. Missing manifest is an empty authority.
func readManifestGated(homeDir string) (state.Manifest, string, error) {
	m, err := state.ReadManifest(homeDir)
	if errors.Is(err, os.ErrNotExist) {
		return state.Manifest{}, "", nil
	}
	if err != nil {
		return state.Manifest{}, "", err
	}
	v, ok := parseVersion(manifestSchemaPrefix, m.Schema)
	if !ok {
		return state.Manifest{}, "", fmt.Errorf("%w: %q", ErrUnknownAuthority, m.Schema)
	}
	if v > supportedVersion {
		return state.Manifest{}, "", fmt.Errorf("%w: %q", ErrUnsupportedNewerSchema, m.Schema)
	}
	return m, state.ComputeBundleDigest(m), nil
}

func parseVersion(prefix, schema string) (int, bool) {
	if !strings.HasPrefix(schema, prefix) {
		return 0, false
	}
	var v int
	if _, err := fmt.Sscanf(strings.TrimPrefix(schema, prefix), "%d", &v); err != nil {
		return 0, false
	}
	return v, true
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// observe returns the target's current durable identity.
func observe(target string) (DigestMode, error) {
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DigestMode{}, nil
		}
		return DigestMode{}, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return DigestMode{}, err
	}
	return DigestMode{Exists: true, SHA256: digestBytes(data), Mode: uint32(info.Mode().Perm())}, nil
}

func (d DigestMode) matches(o DigestMode) bool {
	return d.Exists == o.Exists && d.SHA256 == o.SHA256 && (!o.Exists || d.Mode == o.Mode)
}

func (c *runner) persistPhase(j *Journal, phase Phase) error {
	if err := c.faults.beforeJournal(phase); err != nil {
		return err
	}
	j.LastPhase = phase
	if err := j.persist(c.homeDir); err != nil {
		return err
	}
	return c.faults.afterJournal(phase)
}

// runner carries one Run's mutable accounting.
type runner struct {
	homeDir       string
	plan          Plan
	faults        Faults
	targetWrites  int
	manifestWrite int
}

func (c *runner) result(j *Journal, resumed bool) *Result {
	return &Result{
		TransactionID:  j.TransactionID,
		Resumed:        resumed,
		Phase:          j.LastPhase,
		Status:         j.Status,
		TargetWrites:   c.targetWrites,
		ManifestWrites: c.manifestWrite,
	}
}

func freshRun(ctx context.Context, homeDir string, plan Plan, current state.Manifest, expectedDigest string, faults Faults) (*Result, error) {
	c := &runner{homeDir: homeDir, plan: plan, faults: faults}
	j := &Journal{
		Schema:                 JournalSchema,
		TransactionID:          newTransactionID(),
		HomeDir:                homeDir,
		FromSchema:             state.ManifestSchema,
		ExpectedManifestDigest: expectedDigest,
	}
	for _, r := range plan.Resources {
		j.Resources = append(j.Resources, Resource{
			ID: r.ID, Adapter: r.Adapter, Target: r.Target, Extent: r.Extent,
			Desired: DigestMode{Exists: true, SHA256: digestBytes(r.Desired), Mode: uint32(r.Mode.Perm())},
		})
	}

	// Classification is read-only; the journal is only created when there is
	// work, so a no-op run performs zero writes beyond coordination.
	aligned := true
	for i := range j.Resources {
		r := &j.Resources[i]
		cur, err := observe(r.Target)
		if err != nil {
			return nil, err
		}
		entry, recorded := manifestEntry(current, r.ID)
		switch {
		case cur.Exists && !recorded:
			// Bytes exist with no committed ownership record: unknown owner.
			if err := c.beginAndBlock(j, StatusBlockedUnknownOwnership); err != nil {
				return nil, err
			}
			return c.result(j, false), nil
		}
		r.Before = cur
		if !cur.Exists || !cur.matches(r.Desired) || !entryMatches(entry, recorded, r.Desired.SHA256) {
			aligned = false
		}
	}
	if aligned {
		return &Result{NoOp: true}, nil
	}

	if err := c.persistPhase(j, PhaseDiscovered); err != nil {
		return nil, err
	}
	if err := c.persistPhase(j, PhaseClassified); err != nil {
		return nil, err
	}
	if err := c.snapshot(j); err != nil {
		return nil, err
	}
	if err := c.persistPhase(j, PhaseSnapshotted); err != nil {
		return nil, err
	}
	proposed, proposedDigest, err := buildProposed(current, plan)
	if err != nil {
		return nil, err
	}
	j.ProposedManifestDigest = proposedDigest
	if err := c.persistPhase(j, PhasePrepared); err != nil {
		return nil, err
	}
	_ = state.AppendJournal(homeDir, state.OpIntent, j.TransactionID, strings.Join(resourceIDs(j), ","))
	return c.execute(j, current, proposed, false)
}

// beginAndBlock records a blocked classification durably.
func (c *runner) beginAndBlock(j *Journal, status Status) error {
	if err := c.persistPhase(j, PhaseDiscovered); err != nil {
		return err
	}
	if err := c.faults.beforeJournal(PhaseClassified); err != nil {
		return err
	}
	j.LastPhase = PhaseClassified
	j.Status = status
	if err := j.persist(c.homeDir); err != nil {
		return err
	}
	return c.faults.afterJournal(PhaseClassified)
}

func resumeRun(ctx context.Context, homeDir string, plan Plan, j *Journal, faults Faults) (*Result, error) {
	// A resumed transaction is identified by its journal: the plan must name
	// the same resources by ID and target, whatever their order. Positional
	// alignment could write one resource's desired bytes into another's
	// target before verification caught it, so mismatch fails closed.
	plan, err := alignPlanToJournal(plan, j)
	if err != nil {
		return nil, err
	}
	current, _, err := readManifestGated(homeDir)
	if err != nil {
		return nil, err
	}

	switch j.LastPhase {
	case PhasePrepared, PhaseApplying:
		// Restart decision table: exact before resumes, exact desired skips
		// the write, neither blocks with zero further writes.
		for i := range j.Resources {
			r := &j.Resources[i]
			cur, err := observe(r.Target)
			if err != nil {
				return nil, err
			}
			switch {
			case cur.matches(r.Desired):
				r.Applied = DigestMode{Exists: true, SHA256: r.Desired.SHA256, Mode: r.Desired.Mode}
			case cur.matches(r.Before):
				// resume the unapplied resource below
			default:
				j.Status = StatusBlockedConflict
				if err := j.persist(homeDir); err != nil {
					return nil, err
				}
				return &Result{TransactionID: j.TransactionID, Resumed: true, Phase: j.LastPhase, Status: j.Status}, nil
			}
		}
		proposed, proposedDigest, err := buildProposed(current, plan)
		if err != nil {
			return nil, err
		}
		if j.ProposedManifestDigest != "" && j.ProposedManifestDigest != proposedDigest {
			return nil, fmt.Errorf("%w: proposed digest changed between runs", ErrStaleManifest)
		}
		j.ProposedManifestDigest = proposedDigest
		c := &runner{homeDir: homeDir, plan: plan, faults: faults}
		if j.LastPhase == PhasePrepared {
			if err := c.persistPhase(j, PhaseApplying); err != nil {
				return nil, err
			}
		}
		return c.execute(j, current, proposed, true)

	case PhaseVerified, PhaseManifestCommitted:
		proposed, proposedDigest, err := buildProposed(current, plan)
		if err != nil {
			return nil, err
		}
		if j.ProposedManifestDigest != "" && j.ProposedManifestDigest != proposedDigest {
			return nil, fmt.Errorf("%w: proposed digest changed between runs", ErrStaleManifest)
		}
		j.ProposedManifestDigest = proposedDigest
		c := &runner{homeDir: homeDir, plan: plan, faults: faults}
		if j.LastPhase == PhaseVerified {
			// Re-prove exact desired bytes before publication; a drift here
			// rolls back like any verification failure.
			if err := c.verify(j); err != nil {
				if _, ok := err.(*terminalOutcome); ok {
					return c.result(j, true), nil
				}
				return nil, err
			}
		}
		return c.commit(j, current, proposed, true)

	default:
		return nil, fmt.Errorf("managedstate: journal phase %q is not resumable", j.LastPhase)
	}
}

// snapshot copies before-images into the transaction directory and
// revalidates every referenced digest (snapshotted completes only after the
// backup inventory revalidates).
func (c *runner) snapshot(j *Journal) error {
	dir := filepath.Join(transactionDir(c.homeDir, j.TransactionID), "snapshot")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i := range j.Resources {
		r := &j.Resources[i]
		if !r.Before.Exists {
			continue // absent target: rollback removes the written file
		}
		data, err := os.ReadFile(r.Target)
		if err != nil {
			return err
		}
		if digestBytes(data) != r.Before.SHA256 {
			return fmt.Errorf("managedstate: %s changed between classify and snapshot", r.ID)
		}
		name := fmt.Sprintf("%03d-%s", i, filepath.Base(r.Target))
		if _, err := filemerge.WriteFileAtomic(filepath.Join(dir, name), data, fs.FileMode(r.Before.Mode)); err != nil {
			return err
		}
		reread, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if digestBytes(reread) != r.Before.SHA256 {
			return fmt.Errorf("managedstate: before-image for %s failed revalidation", r.ID)
		}
		r.Snapshot = name
	}
	return nil
}

// alignPlanToJournal rebuilds the plan in journal order, matching resources
// by ID. A journal resource without a plan counterpart, or a plan resource
// pointing at a different target, is a fail-closed mismatch: the durable
// journal owns the identity of what is being resumed.
func alignPlanToJournal(plan Plan, j *Journal) (Plan, error) {
	byID := make(map[string]PlannedResource, len(plan.Resources))
	for _, r := range plan.Resources {
		byID[r.ID] = r
	}
	journalIDs := make(map[string]bool, len(j.Resources))
	for _, jr := range j.Resources {
		journalIDs[jr.ID] = true
	}
	for _, r := range plan.Resources {
		if !journalIDs[r.ID] {
			return Plan{}, fmt.Errorf("managedstate: plan names resource %q absent from the resumed transaction; complete this transaction, then run new resources as their own", r.ID)
		}
	}
	aligned := Plan{Producer: plan.Producer, Resources: make([]PlannedResource, len(j.Resources))}
	for i, jr := range j.Resources {
		pr, ok := byID[jr.ID]
		if !ok {
			return Plan{}, fmt.Errorf("managedstate: resumed transaction names resource %q absent from the plan", jr.ID)
		}
		if pr.Target != jr.Target {
			return Plan{}, fmt.Errorf("managedstate: resumed resource %q target changed from %q to %q", jr.ID, jr.Target, pr.Target)
		}
		aligned.Resources[i] = pr
	}
	return aligned, nil
}

// execute drives applying -> verified -> manifest_committed -> completed.
func (c *runner) execute(j *Journal, current state.Manifest, proposed state.Manifest, resumed bool) (*Result, error) {
	if phaseOrder[j.LastPhase] < phaseOrder[PhaseApplying] {
		if err := c.persistPhase(j, PhaseApplying); err != nil {
			return nil, err
		}
	}
	if err := c.apply(j, resumed); err != nil {
		if _, ok := err.(*terminalOutcome); ok {
			return c.result(j, resumed), nil
		}
		return nil, err
	}
	if err := c.persistPhase(j, PhaseApplying); err != nil {
		return nil, err
	}
	if err := c.verify(j); err != nil {
		if _, ok := err.(*terminalOutcome); ok {
			return c.result(j, resumed), nil
		}
		return nil, err
	}
	if err := c.persistPhase(j, PhaseVerified); err != nil {
		return nil, err
	}
	return c.commit(j, current, proposed, resumed)
}

// terminalOutcome marks a run that ended in a recorded Status.
type terminalOutcome struct{ status Status }

func (t *terminalOutcome) Error() string { return "managedstate: terminal " + string(t.status) }

// apply writes desired bytes once, refusing to touch foreign content.
func (c *runner) apply(j *Journal, resumed bool) error {
	for i := range j.Resources {
		r := &j.Resources[i]
		if r.Applied.Exists && r.Applied.SHA256 == r.Desired.SHA256 {
			continue // already exact desired (crash-after-replace resume)
		}
		cur, err := observe(r.Target)
		if err != nil {
			return err
		}
		if r.Before.Exists {
			if cur.Exists && !cur.matches(r.Before) {
				// Foreign bytes after snapshot. Live run: rollback_required,
				// never a guessed overwrite. Restart classification happens
				// in the decision table before reaching here.
				j.Status = StatusRollbackRequired
				if err := j.persist(c.homeDir); err != nil {
					return err
				}
				return &terminalOutcome{status: StatusRollbackRequired}
			}
		} else if cur.Exists {
			// The target was absent at snapshot and bytes appeared since:
			// someone else's content, same foreign-writer refusal.
			j.Status = StatusRollbackRequired
			if err := j.persist(c.homeDir); err != nil {
				return err
			}
			return &terminalOutcome{status: StatusRollbackRequired}
		}
		if cur.Exists && cur.matches(r.Desired) {
			r.Applied = cur // already exact desired bytes and mode
			continue
		}
		if err := c.faults.beforeTargetWrite(r.Target); err != nil {
			return err
		}
		if _, err := filemerge.WriteFileAtomicMode(r.Target, c.plan.Resources[i].Desired, c.plan.Resources[i].Mode); err != nil {
			return err
		}
		c.targetWrites++
		r.Applied = DigestMode{Exists: true, SHA256: r.Desired.SHA256, Mode: r.Desired.Mode}
		if err := c.faults.afterTargetWrite(r.Target); err != nil {
			return err
		}
	}
	return nil
}

// verify proves every target reads back as exact desired, else rolls back.
func (c *runner) verify(j *Journal) error {
	for i := range j.Resources {
		r := &j.Resources[i]
		if err := c.faults.verify(r.Target); err != nil {
			return c.rollback(j)
		}
		cur, err := observe(r.Target)
		if err != nil {
			return err
		}
		if !cur.matches(r.Desired) {
			return c.rollback(j)
		}
		r.Applied = cur
	}
	return nil
}

// rollback is the explicit transition: restore every before-image, and only
// report rolled_back after a byte-and-mode exact re-read. Partial restore
// stays rollback_required; it is never reported as rolled back.
func (c *runner) rollback(j *Journal) error {
	dir := filepath.Join(transactionDir(c.homeDir, j.TransactionID), "snapshot")
	for i := range j.Resources {
		r := &j.Resources[i]
		if r.Snapshot == "" {
			if r.Applied.Exists {
				if err := os.Remove(r.Target); err != nil && !errors.Is(err, os.ErrNotExist) {
					j.Status = StatusRollbackRequired
					_ = j.persist(c.homeDir)
					return &terminalOutcome{status: StatusRollbackRequired}
				}
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, r.Snapshot))
		if err != nil {
			j.Status = StatusRollbackRequired
			_ = j.persist(c.homeDir)
			return &terminalOutcome{status: StatusRollbackRequired}
		}
		if _, err := filemerge.WriteFileAtomicMode(r.Target, data, fs.FileMode(r.Before.Mode)); err != nil {
			j.Status = StatusRollbackRequired
			_ = j.persist(c.homeDir)
			return &terminalOutcome{status: StatusRollbackRequired}
		}
	}
	for i := range j.Resources {
		r := &j.Resources[i]
		cur, err := observe(r.Target)
		if err != nil || !cur.matches(r.Before) {
			j.Status = StatusRollbackRequired
			_ = j.persist(c.homeDir)
			return &terminalOutcome{status: StatusRollbackRequired}
		}
	}
	j.Status = StatusRolledBack
	if err := j.persist(c.homeDir); err != nil {
		return err
	}
	return &terminalOutcome{status: StatusRolledBack}
}

// publish writes the proposed manifest exactly once with fault seams at
// every boundary and verifies the readback digest.
func (c *runner) publish(j *Journal, proposed state.Manifest) error {
	if err := c.faults.beforeManifestPublish(); err != nil {
		return err
	}
	if err := state.WriteManifestAtomic(c.homeDir, proposed); err != nil {
		return err
	}
	c.manifestWrite++
	readback, err := state.ReadManifest(c.homeDir)
	if err != nil {
		return err
	}
	if state.ComputeBundleDigest(readback) != j.ProposedManifestDigest {
		return errManifestWriteVerify
	}
	return c.faults.afterManifestPublish()
}

// observedSettled reports whether every planned resource's committed entry
// records observed == desired. ComputeBundleDigest deliberately ignores
// observed, so identity equality alone cannot prove the metadata settled.
func observedSettled(m state.Manifest, plan Plan) bool {
	for _, r := range plan.Resources {
		entry, ok := manifestEntry(m, r.ID)
		if !ok || entry.Desired != digestBytes(r.Desired) || entry.Observed != digestBytes(r.Desired) {
			return false
		}
	}
	return true
}

// commit publishes the proposed manifest under compare-and-swap, or, when
// this transaction already published it before a crash, finishes bookkeeping
// only after proving the committed generation is exactly ours.
func (c *runner) commit(j *Journal, current state.Manifest, proposed state.Manifest, resumed bool) (*Result, error) {
	onDisk, err := state.ReadManifest(c.homeDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	digest := ""
	if !errors.Is(err, os.ErrNotExist) {
		digest = state.ComputeBundleDigest(onDisk)
	}

	switch {
	case digest == j.ProposedManifestDigest && observedSettled(onDisk, c.plan):
		// The committed generation is exactly ours and the per-resource
		// observed metadata is settled: either this transaction already
		// persisted manifest_committed before a crash, or it wrote the
		// manifest and crashed before the journal revision landed. Both
		// finish with bookkeeping only; the manifest is never rewritten.
	case digest == j.ProposedManifestDigest:
		// The canonical digest ignores observed metadata, so our identity can
		// sit above stale observed fields. verify() proved the targets exact:
		// republish the verified generation once to settle the metadata a
		// later no-op depends on, then continue as bookkeeping.
		if err := c.publish(j, proposed); err != nil {
			return nil, err
		}
	case j.LastPhase == PhaseManifestCommitted:
		// Decision-table stale branch: a committed journal whose on-disk
		// generation is not this transaction's is a typed stale-CAS.
		return nil, fmt.Errorf("%w: committed generation %q is not this transaction's", ErrStaleManifest, digest)
	default:
		// CAS: the manifest generation this transaction started from must
		// still be the committed one.
		if digest != j.ExpectedManifestDigest {
			return nil, fmt.Errorf("%w: expected %q found %q", ErrStaleManifest, j.ExpectedManifestDigest, digest)
		}
		if state.ComputeBundleDigest(proposed) != j.ProposedManifestDigest {
			return nil, fmt.Errorf("%w: proposed digest drifted", ErrStaleManifest)
		}
		if err := c.publish(j, proposed); err != nil {
			return nil, err
		}
	}
	if j.LastPhase != PhaseManifestCommitted {
		if err := c.persistPhase(j, PhaseManifestCommitted); err != nil {
			return nil, err
		}
	}

	// completed: the journal never survives as a second committed truth.
	// The audit line is best-effort observability after the durable commit;
	// its failure must not resurrect the transaction.
	_ = state.AppendJournal(c.homeDir, state.OpComplete, j.TransactionID, strings.Join(resourceIDs(j), ","))
	if err := os.RemoveAll(transactionDir(c.homeDir, j.TransactionID)); err != nil {
		return nil, err
	}
	// Leave no trace when this was the only transaction.
	_ = os.Remove(TransactionsDir(c.homeDir))
	j.LastPhase = PhaseCompleted
	return c.result(j, resumed), nil
}

func resourceIDs(j *Journal) []string {
	ids := make([]string, len(j.Resources))
	for i, r := range j.Resources {
		ids[i] = r.ID
	}
	return ids
}

func manifestEntry(m state.Manifest, id string) (state.ManifestResource, bool) {
	for _, r := range m.Resources {
		if r.ID == id {
			return r, true
		}
	}
	return state.ManifestResource{}, false
}

func entryMatches(entry state.ManifestResource, recorded bool, desiredDigest string) bool {
	return recorded && entry.Desired == desiredDigest && entry.Observed == desiredDigest
}

func buildProposed(current state.Manifest, plan Plan) (state.Manifest, string, error) {
	proposed := state.Manifest{
		Schema:    state.ManifestSchema,
		Producer:  plan.Producer,
		Resources: append([]state.ManifestResource(nil), current.Resources...),
	}
	for _, r := range plan.Resources {
		entry, _ := manifestEntry(proposed, r.ID)
		entry.ID = r.ID
		entry.Adapter = r.Adapter
		entry.Target = r.Target
		entry.OwnedExtent = r.Extent
		entry.Desired = digestBytes(r.Desired)
		entry.Observed = digestBytes(r.Desired)
		if idx := indexOfID(proposed.Resources, r.ID); idx >= 0 {
			proposed.Resources[idx] = entry
		} else {
			proposed.Resources = append(proposed.Resources, entry)
		}
	}
	proposed = proposed.WithBundleDigest()
	return proposed, state.ComputeBundleDigest(proposed), nil
}

func indexOfID(rs []state.ManifestResource, id string) int {
	for i, r := range rs {
		if r.ID == id {
			return i
		}
	}
	return -1
}
