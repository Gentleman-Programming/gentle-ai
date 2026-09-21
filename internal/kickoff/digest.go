package kickoff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
)

// ArtifactDigest computes a stable SHA-256 digest over the given artifact
// paths, in sorted path order regardless of the order the caller supplied
// them in — two calls naming the same set of artifacts always produce the
// same digest. Line endings are normalized (CRLF to LF) before hashing, so
// the same logical content checked out on different platforms produces the
// same digest — a gate's approval must not spuriously reopen just because a
// checkout normalized line endings or a caller iterated a map.
//
// An unreadable path is a named error: ArtifactDigest never substitutes a
// silent zero-value digest for a missing artifact (D-08 depends on the
// digest being trustworthy evidence of "did the artifact actually
// change").
func ArtifactDigest(paths []string) (string, error) {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)

	hasher := sha256.New()
	for _, path := range sorted {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("leer %q para el digest del artefacto: %w", path, err)
		}
		hasher.Write(normalizeLineEndings(data))
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// normalizeLineEndings rewrites CRLF to LF so digest computation is
// independent of the checkout's line-ending configuration.
func normalizeLineEndings(data []byte) []byte {
	normalized := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			continue
		}
		normalized = append(normalized, data[i])
	}
	return normalized
}
