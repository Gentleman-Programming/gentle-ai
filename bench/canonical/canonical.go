package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// Result is the path-free canonical projection of a raw bench result.
// It replaces sandbox absolute paths with typed tokens so that equivalent
// runs across different machines produce identical bytes and therefore
// identical sha256 digests.
type Result struct {
	Identity struct {
		TargetBinarySHA256  string `json:"target_binary_sha256"`
		EmbeddedVCSRevision string `json:"embedded_vcs_revision"`
		ClassifierVersion   string `json:"classifier_version"`
		SourceRevision      string `json:"source_revision"`
		VCSModified         bool   `json:"vcs_modified"`
		RuntimeGOOS         string `json:"runtime_goos"`
		RuntimeGOARCH       string `json:"runtime_goarch"`
	} `json:"identity"`
	Corpus     map[string]string `json:"corpus"`
	Invocation struct {
		Mode string   `json:"mode"`
		Only []string `json:"only,omitempty"`
	} `json:"invocation"`
	Outcomes struct {
		Completed   int `json:"completed"`
		Failed      int `json:"failed"`
		Unsupported int `json:"unsupported"`
		InBand      int `json:"in_band"`
		OutOfBand   int `json:"out_of_band"`
		DeadEnd     int `json:"dead_end"`
	} `json:"outcomes"`
}

// Canonicalize projects a raw bench Result into a path-free canonical form.
// Sandbox paths are replaced with typed tokens: <RUN_ROOT>, <TARGET_BINARY>,
// <REPOSITORY>, <HOME>. The transformation is deterministic: identical inputs
// across isolated runs produce byte-for-byte identical output.
//
// The raw argument is the JSON bytes of the raw bench metrics.Result.
func Canonicalize(raw []byte) ([]byte, error) {
	// Normalise Windows separators to forward slashes.
	norm := strings.ReplaceAll(string(raw), "\\", "/")

	// Replace sandbox absolute paths with typed tokens, longest-first.
	norm = replaceSandboxPaths(norm)

	// Unmarshal the path-replaced JSON.
	var data map[string]any
	if err := json.Unmarshal([]byte(norm), &data); err != nil {
		return nil, err
	}

	result := Result{}

	// Identity
	if id, ok := data["identity"].(map[string]any); ok {
		if v, ok := id["target_binary_sha256"].(string); ok {
			result.Identity.TargetBinarySHA256 = v
		}
		if v, ok := id["embedded_vcs_revision"].(string); ok {
			result.Identity.EmbeddedVCSRevision = v
		}
		if v, ok := id["classifier_version"].(string); ok {
			result.Identity.ClassifierVersion = v
		}
		if v, ok := id["source_revision"].(string); ok {
			result.Identity.SourceRevision = v
		}
		if v, ok := id["vcs_modified"].(bool); ok {
			result.Identity.VCSModified = v
		}
		if v, ok := id["runtime_goos"].(string); ok {
			result.Identity.RuntimeGOOS = v
		}
		if v, ok := id["runtime_goarch"].(string); ok {
			result.Identity.RuntimeGOARCH = v
		}
	}

	// Corpus — map with sorted keys for determinism.
	if corpus, ok := data["corpus"].(map[string]any); ok {
		result.Corpus = make(map[string]string)
		keys := make([]string, 0, len(corpus))
		for k := range corpus {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v, ok := corpus[k].(string); ok {
				result.Corpus[k] = v
			}
		}
	}

	// Invocation
	if inv, ok := data["invocation"].(map[string]any); ok {
		if v, ok := inv["mode"].(string); ok {
			result.Invocation.Mode = v
		}
		if only, ok := inv["only"].([]any); ok {
			result.Invocation.Only = make([]string, 0, len(only))
			for _, o := range only {
				if s, ok := o.(string); ok {
					result.Invocation.Only = append(result.Invocation.Only, s)
				}
			}
			sort.Strings(result.Invocation.Only)
		}
	}

	// Outcomes — aggregate journey-level status and block counts.
	if journeys, ok := data["journeys"].([]any); ok {
		for _, j := range journeys {
			if jm, ok := j.(map[string]any); ok {
				switch jm["status"] {
				case "completed":
					result.Outcomes.Completed++
				case "failed":
					result.Outcomes.Failed++
				case "unsupported":
					result.Outcomes.Unsupported++
				}
				if metrics, ok := jm["metrics"].(map[string]any); ok {
					if blocks, ok := metrics["blocks"].(map[string]any); ok {
						if v, ok := toInt(blocks["in_band"]); ok {
							result.Outcomes.InBand += v
						}
						if v, ok := toInt(blocks["out_of_band"]); ok {
							result.Outcomes.OutOfBand += v
						}
						if v, ok := toInt(blocks["dead_end"]); ok {
							result.Outcomes.DeadEnd += v
						}
					}
				}
			}
		}
	}

	return json.Marshal(result)
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

// Digest returns the sha256 hex digest of the canonical bytes with the "sha256:"
// prefix, matching the contract: sha256:<lowercase-hex>.
func Digest(canonical []byte) string {
	h := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(h[:])
}

// replaceSandboxPaths replaces sandbox absolute paths with typed tokens.
// Tokens applied longest-first to prevent partial substitution of nested paths.
func replaceSandboxPaths(s string) string {
	// Ordered longest-first. Each entry: regex pattern, replacement token.
	// The identifier part [a-zA-Z0-9._-]+ captures sandbox IDs that may contain dots.
	replacements := []struct {
		pattern *regexp.Regexp
		token   string
	}{
		{regexp.MustCompile(`/tmp/gentle-ai-bench-[a-zA-Z0-9._-]+/home/demo`), "<REPOSITORY>"},
		{regexp.MustCompile(`/tmp/gentle-ai-bench-[a-zA-Z0-9._-]+/home`), "<HOME>"},
		{regexp.MustCompile(`/tmp/gentle-ai-bench-[a-zA-Z0-9._-]+/gentle-ai-test(?:[.]exe)?`), "<TARGET_BINARY>"},
		{regexp.MustCompile(`/tmp/gentle-ai-bench-[a-zA-Z0-9._-]+`), "<RUN_ROOT>"},
	}
	for _, r := range replacements {
		s = r.pattern.ReplaceAllString(s, r.token)
	}
	return s
}
