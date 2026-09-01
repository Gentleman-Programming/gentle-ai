package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// A reviewer result that admission refuses used to be discarded, so a defect
// report could only quote the refusal and never the bytes that earned it
// (issues #3942, #2791). Every refused payload is now preserved verbatim under
// <GitCommonDir>/gentle-ai/rejected-results/<lineage>/, outside the working
// tree and outside the review-transactions authority store: it is evidence,
// never authority, and nothing reads it back into a lineage.
const (
	reviewRejectedResultSchema  = "gentle-ai.review-rejected-result/v1"
	reviewRejectedResultDirName = "rejected-results"
)

type reviewRejectedResultMeta struct {
	LineageID string
	Lens      string
	Attempt   int
	Reason    string
}

type reviewRejectedResultEnvelope struct {
	Schema     string `json:"schema"`
	LineageID  string `json:"lineage_id"`
	Lens       string `json:"lens"`
	Attempt    int    `json:"attempt"`
	Reason     string `json:"reason"`
	RawSHA256  string `json:"raw_sha256"`
	CapturedAt string `json:"captured_at"`
	Raw        string `json:"raw"`
}

// writeReviewRejectedResult persists one refused payload and returns its path.
func writeReviewRejectedResult(ctx context.Context, root string, meta reviewRejectedResultMeta, raw []byte) (string, error) {
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(ctx, root)
	if err != nil {
		return "", fmt.Errorf("resolve repository identity for rejected result: %w", err)
	}
	digest := sha256.Sum256(raw)
	rawSHA256 := hex.EncodeToString(digest[:])
	dir := filepath.Join(lease.Identity().GitCommonDir, "gentle-ai", reviewRejectedResultDirName, reviewRejectedResultPathComponent(meta.LineageID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create rejected result directory: %w", err)
	}
	content, err := json.Marshal(reviewRejectedResultEnvelope{
		Schema: reviewRejectedResultSchema, LineageID: meta.LineageID, Lens: meta.Lens, Attempt: meta.Attempt,
		Reason: meta.Reason, RawSHA256: rawSHA256, CapturedAt: time.Now().UTC().Format(time.RFC3339), Raw: string(raw),
	})
	if err != nil {
		return "", fmt.Errorf("encode rejected result: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%d-%s.json", reviewRejectedResultPathComponent(meta.Lens), meta.Attempt, rawSHA256[:12]))
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write rejected result: %w", err)
	}
	return path, nil
}

// reviewRejectedResultPathComponent keeps a lineage or lens identifier inside
// one path component.
func reviewRejectedResultPathComponent(value string) string {
	var component strings.Builder
	for _, r := range value {
		if r == '-' || r == '.' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			component.WriteRune(r)
		} else {
			component.WriteByte('_')
		}
	}
	if component.Len() == 0 {
		return "unknown"
	}
	return component.String()
}

// reviewRejectedResultClause preserves the payload and returns the clause a
// refusal appends. Persistence failure degrades to an empty clause, exactly
// like reviewGenerateToolFaultDefectReport: the original refusal is always
// what the caller sees, never a second unrelated error.
func reviewRejectedResultClause(ctx context.Context, root string, meta reviewRejectedResultMeta, raw []byte) string {
	path, err := writeReviewRejectedResult(ctx, root, meta, raw)
	if err != nil {
		return ""
	}
	return "; the rejected reviewer payload was preserved at " + path
}
