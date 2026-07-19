package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

const ChainBundleSchema = "gentle-ai.review-chain-bundle/v1"

type ChainBundleEvent struct {
	Revision string `json:"revision"`
	Payload  []byte `json:"payload"`
}

type ChainBundle struct {
	Schema                  string             `json:"schema"`
	LineageID               string             `json:"lineage_id"`
	Generation              int                `json:"generation"`
	InitialSnapshotIdentity string             `json:"initial_snapshot_identity"`
	FinalSnapshotIdentity   string             `json:"final_snapshot_identity"`
	PolicyHash              string             `json:"policy_hash"`
	LedgerHash              string             `json:"ledger_hash,omitempty"`
	EvidenceHash            string             `json:"evidence_hash,omitempty"`
	GenesisRevision         string             `json:"genesis_revision"`
	HeadRevision            string             `json:"head_revision"`
	ChainIdentity           string             `json:"chain_identity"`
	TerminalReceipt         *Receipt           `json:"terminal_receipt,omitempty"`
	Events                  []ChainBundleEvent `json:"events"`
	BundleDigest            string             `json:"bundle_digest"`
}

type BundleImportExpectation struct {
	LineageID       string
	Snapshot        Snapshot
	PolicyHash      string
	LedgerHash      string
	EvidenceHash    string
	FixDeltaHash    string
	Receipt         Receipt
	GenesisRevision string
	HeadRevision    string
	ChainIdentity   string
	BundleDigest    string
}

func (store Store) ExportBundle() (ChainBundle, error) {
	chain, err := store.LoadChain()
	if err != nil {
		return ChainBundle{}, err
	}
	events := make([]ChainBundleEvent, len(chain.Revisions))
	for index, revision := range chain.Revisions {
		payload, err := os.ReadFile(filepath.Join(store.Dir, "events", strings.TrimPrefix(revision, "sha256:")+".json"))
		if err != nil {
			return ChainBundle{}, err
		}
		events[index] = ChainBundleEvent{Revision: revision, Payload: payload}
	}
	genesis := chain.Records[0].Transaction
	terminal := chain.Records[len(chain.Records)-1].Transaction
	var receipt *Receipt
	if terminal.State == StateApproved || terminal.State == StateEscalated {
		value, err := terminal.Receipt()
		if err != nil {
			return ChainBundle{}, err
		}
		receipt = &value
	}
	bundle := ChainBundle{
		Schema: ChainBundleSchema, LineageID: terminal.LineageID, Generation: terminal.Generation,
		InitialSnapshotIdentity: genesis.Snapshot.Identity, FinalSnapshotIdentity: terminal.Snapshot.Identity,
		PolicyHash: terminal.PolicyHash, LedgerHash: terminal.LedgerHash, EvidenceHash: terminal.EvidenceHash,
		GenesisRevision: chain.GenesisRevision, HeadRevision: chain.HeadRevision, ChainIdentity: chain.Identity,
		TerminalReceipt: receipt, Events: events,
	}
	bundle.BundleDigest = bundleDigest(bundle)
	return bundle, nil
}

func ParseChainBundle(payload []byte) (ChainBundle, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var bundle ChainBundle
	if err := decoder.Decode(&bundle); err != nil {
		return ChainBundle{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ChainBundle{}, errors.New("multiple JSON values in review chain bundle")
	}
	if _, err := validateChainBundle(bundle); err != nil {
		return ChainBundle{}, err
	}
	return bundle, nil
}

func WriteChainBundleAtomic(path string, bundle ChainBundle) error {
	if _, err := validateChainBundle(bundle); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(payload, '\n'), 0o644)
}

func ImportBundle(ctx context.Context, repo string, bundle ChainBundle, expected BundleImportExpectation) (ValidatedChain, error) {
	chain, err := validateChainBundle(bundle)
	if err != nil {
		return ValidatedChain{}, fmt.Errorf("validate review chain bundle: %w", err)
	}
	if err := validateBundleExpectation(ctx, repo, bundle, chain, expected); err != nil {
		return ValidatedChain{}, err
	}
	store, err := AuthoritativeStore(ctx, repo, expected.LineageID)
	if err != nil {
		return ValidatedChain{}, fmt.Errorf("derive bundle destination: %w", err)
	}
	return store.installBundle(chain, bundle.Events)
}

func validateChainBundle(bundle ChainBundle) (ValidatedChain, error) {
	if bundle.Schema != ChainBundleSchema {
		return ValidatedChain{}, errors.New("unsupported review chain bundle schema")
	}
	if err := validateLineageID(bundle.LineageID); err != nil {
		return ValidatedChain{}, err
	}
	if bundle.Generation < 1 || !validSHA256(bundle.InitialSnapshotIdentity) || !validSHA256(bundle.FinalSnapshotIdentity) || !validSHA256(bundle.PolicyHash) || !validSHA256(bundle.GenesisRevision) || !validSHA256(bundle.HeadRevision) || !validSHA256(bundle.ChainIdentity) || !validSHA256(bundle.BundleDigest) {
		return ValidatedChain{}, errors.New("review chain bundle metadata is incomplete")
	}
	if bundle.LedgerHash != "" && !validSHA256(bundle.LedgerHash) {
		return ValidatedChain{}, errors.New("review chain bundle ledger hash is invalid")
	}
	if bundle.EvidenceHash != "" && !validSHA256(bundle.EvidenceHash) {
		return ValidatedChain{}, errors.New("review chain bundle evidence hash is invalid")
	}
	if bundle.Events == nil || len(bundle.Events) == 0 {
		return ValidatedChain{}, errors.New("review chain bundle requires the complete ordered event list")
	}
	if bundle.BundleDigest != bundleDigest(bundle) {
		return ValidatedChain{}, errors.New("review chain bundle digest mismatch")
	}

	records := make([]Record, len(bundle.Events))
	revisions := make([]string, len(bundle.Events))
	for index, event := range bundle.Events {
		if !validSHA256(event.Revision) || len(event.Payload) == 0 {
			return ValidatedChain{}, fmt.Errorf("bundle event[%d] is incomplete", index)
		}
		sum := sha256.Sum256(event.Payload)
		if "sha256:"+hex.EncodeToString(sum[:]) != event.Revision {
			return ValidatedChain{}, fmt.Errorf("bundle event[%d] hash mismatch", index)
		}
		record, err := parseRecordPayload(event.Payload)
		if err != nil {
			return ValidatedChain{}, fmt.Errorf("bundle event[%d]: %w", index, err)
		}
		records[index] = record
		revisions[index] = event.Revision
	}
	if !validInitialStoreRecord(records[0]) || records[0].PreviousRevision != "" {
		return ValidatedChain{}, errors.New("review chain bundle does not start at one legal genesis")
	}
	for index := 1; index < len(records); index++ {
		if records[index].PreviousRevision != revisions[index-1] {
			return ValidatedChain{}, errors.New("review chain bundle predecessor is incomplete or reordered")
		}
		if err := validatePersistedV1Successor(records[index-1].Transaction, records[index].Transaction, records[index].Operation, index); err != nil {
			return ValidatedChain{}, fmt.Errorf("review chain bundle successor[%d]: %w", index, err)
		}
	}
	chain := ValidatedChain{
		Records: records, Revisions: revisions, GenesisRevision: revisions[0],
		HeadRevision: revisions[len(revisions)-1], Identity: chainIdentity(revisions),
	}
	terminal := records[len(records)-1].Transaction
	if chain.GenesisRevision != bundle.GenesisRevision || chain.HeadRevision != bundle.HeadRevision || chain.Identity != bundle.ChainIdentity || records[0].Transaction.LineageID != bundle.LineageID || terminal.LineageID != bundle.LineageID || terminal.Generation != bundle.Generation || records[0].Transaction.Snapshot.Identity != bundle.InitialSnapshotIdentity || terminal.Snapshot.Identity != bundle.FinalSnapshotIdentity || terminal.PolicyHash != bundle.PolicyHash || terminal.LedgerHash != bundle.LedgerHash || terminal.EvidenceHash != bundle.EvidenceHash {
		return ValidatedChain{}, errors.New("review chain bundle metadata does not match its event chain")
	}
	if bundle.TerminalReceipt != nil {
		receipt, err := terminal.Receipt()
		if err != nil || !reflect.DeepEqual(receipt, *bundle.TerminalReceipt) {
			return ValidatedChain{}, errors.New("review chain bundle receipt does not match its terminal event")
		}
	}
	return chain, nil
}

func validateBundleExpectation(ctx context.Context, repo string, bundle ChainBundle, chain ValidatedChain, expected BundleImportExpectation) error {
	if expected.LineageID != bundle.LineageID || expected.GenesisRevision != bundle.GenesisRevision || expected.HeadRevision != bundle.HeadRevision || expected.ChainIdentity != bundle.ChainIdentity || expected.BundleDigest != bundle.BundleDigest {
		return errors.New("review chain bundle does not match the expected chain identity")
	}
	if genesis := chain.Records[0].Transaction; genesis.Mode == ModeOrdinaryBounded && !genesis.hasCorrectionBudget() {
		return errors.New("portable ordinary_bounded bundle requires correction budget fields")
	}
	if err := validateSnapshot(expected.Snapshot); err != nil {
		return fmt.Errorf("expected current snapshot: %w", err)
	}
	terminal := chain.Records[len(chain.Records)-1].Transaction
	if err := validateRepositoryBudgetEvidence(ctx, repo, chain.Records); err != nil {
		return fmt.Errorf("review chain bundle: %w", err)
	}
	if bundle.TerminalReceipt != nil {
		if err := validateReceiptStructure(expected.Receipt); err != nil {
			return fmt.Errorf("expected receipt: %w", err)
		}
		receipt, err := terminal.Receipt()
		if err != nil || !reflect.DeepEqual(receipt, expected.Receipt) || !reflect.DeepEqual(*bundle.TerminalReceipt, expected.Receipt) {
			return errors.New("review chain bundle does not match the expected terminal receipt")
		}
		if expected.LineageID != receipt.LineageID || receipt.LineageID != terminal.LineageID || receipt.Generation != terminal.Generation || receipt.Mode != terminal.Mode {
			return errors.New("review chain bundle does not match the expected lineage, generation, or mode")
		}
	}
	if expected.Snapshot.CandidateTree != terminal.FinalCandidateTree || expected.Snapshot.PathsDigest != terminal.PathsDigest {
		return errors.New("review chain bundle does not match current delivered content or path scope")
	}
	intendedProof, err := (SnapshotBuilder{Repo: repo}).untrackedProof(ctx, expected.Snapshot.CandidateTree, terminal.Snapshot.IntendedUntracked)
	if err != nil || intendedProof != terminal.Snapshot.IntendedUntrackedProof {
		return errors.New("review chain bundle does not match the authoritative intended-untracked proof")
	}
	if expected.PolicyHash != terminal.PolicyHash || expected.LedgerHash != terminal.LedgerHash || expected.EvidenceHash != terminal.EvidenceHash || (expected.FixDeltaHash != "" && expected.FixDeltaHash != terminal.FixDeltaHash) {
		return errors.New("review chain bundle does not match current policy, ledger, evidence, or fix delta")
	}
	return nil
}

func (store Store) installBundle(chain ValidatedChain, events []ChainBundleEvent) (ValidatedChain, error) {
	if strings.TrimSpace(store.Dir) == "" || len(events) != len(chain.Revisions) {
		return ValidatedChain{}, errors.New("review bundle destination is invalid")
	}
	if err := os.MkdirAll(filepath.Join(store.Dir, "events"), 0o755); err != nil {
		return ValidatedChain{}, err
	}
	lock, err := acquireStoreLock(filepath.Join(store.Dir, "LOCK"))
	if err != nil {
		return ValidatedChain{}, err
	}
	defer lock.release()
	current, err := readRevision(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return ValidatedChain{}, err
	}
	if current != "" {
		loaded, err := store.loadChain(current)
		if err == nil && current == chain.HeadRevision && loaded.Identity == chain.Identity {
			return loaded, nil
		}
		return ValidatedChain{}, ErrConcurrentUpdate
	}
	for index, event := range events {
		if event.Revision != chain.Revisions[index] {
			return ValidatedChain{}, errors.New("review bundle installation order changed")
		}
		path := filepath.Join(store.Dir, "events", strings.TrimPrefix(event.Revision, "sha256:")+".json")
		if err := installContentAddressedFile(path, event.Payload); err != nil {
			return ValidatedChain{}, err
		}
	}
	if err := writeAtomic(filepath.Join(store.Dir, "HEAD"), []byte(chain.HeadRevision+"\n"), 0o644); err != nil {
		return ValidatedChain{}, err
	}
	return store.loadChain(chain.HeadRevision)
}

func installContentAddressedFile(path string, payload []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".bundle-event-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(payload); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := publishNoReplace(tempPath, path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, payload) {
			return errors.New("existing content-addressed bundle event differs")
		}
	}
	return nil
}

func parseRecordPayload(payload []byte) (Record, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record Record
	if err := decoder.Decode(&record); err != nil {
		return Record{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Record{}, errors.New("multiple JSON values in review record")
	}
	if record.Schema != RecordSchema || strings.TrimSpace(record.Operation) == "" {
		return Record{}, errors.New("invalid review record")
	}
	if err := record.Transaction.validate(); err != nil {
		return Record{}, err
	}
	return record, nil
}

func bundleDigest(bundle ChainBundle) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("gentle-ai.review-chain-bundle/v1\x00"))
	for _, value := range []string{
		bundle.Schema, bundle.LineageID, strconv.Itoa(bundle.Generation), bundle.InitialSnapshotIdentity,
		bundle.FinalSnapshotIdentity, bundle.PolicyHash, bundle.LedgerHash, bundle.EvidenceHash,
		bundle.GenesisRevision, bundle.HeadRevision, bundle.ChainIdentity,
	} {
		writeLengthPrefixed(hash, []byte(value))
	}
	if bundle.TerminalReceipt != nil {
		payload, _ := json.Marshal(bundle.TerminalReceipt)
		writeLengthPrefixed(hash, payload)
	} else {
		writeLengthPrefixed(hash, nil)
	}
	for _, event := range bundle.Events {
		writeLengthPrefixed(hash, []byte(event.Revision))
		writeLengthPrefixed(hash, event.Payload)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}
