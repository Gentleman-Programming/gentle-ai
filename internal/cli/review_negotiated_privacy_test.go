package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// negotiatedPrivateFieldPath walks a decoded negotiated payload and reports the
// path of the first key that names a provider-private handle. It exists because
// a key-name-only check cannot answer the question it is actually asked.
//
// The invariant being protected is real and unchanged: negotiated output must
// never carry a provider-private handle -- a store path, an authority path, a
// lock, a directory, a credential token. The problem is that "token" names two
// unrelated things. One is such a handle. The other is a published property of
// $defs/transition_argument in the status schemas, holding the literal argv
// string ("--lineage=review-abc") a caller pastes into a command. Forbidding
// the key everywhere rejects the published contract; allowing it everywhere
// would let a real handle through under a borrowed name.
//
// So this walker discriminates by location instead of by name. A forbidden key
// is a violation everywhere except at the exact paths the published schema
// declares it, and even there only when the value is shaped like the argv the
// schema promises. Every other position -- including a "token" one level up, or
// a sibling of the published one -- is still reported. That is strictly
// narrower than "the key is banned outright" only for the positions the
// contract itself publishes, and strictly wider than a name check for
// everything else, because the reported path now says where the leak is.
func negotiatedPrivateFieldPath(value any, forbidden map[string]struct{}, published map[string]func(any) bool) string {
	return walkNegotiatedPrivateField(value, "", forbidden, published)
}

func walkNegotiatedPrivateField(value any, path string, forbidden map[string]struct{}, published map[string]func(any) bool) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if _, private := forbidden[strings.ToLower(key)]; private && !negotiatedFieldIsPublished(childPath, child, published) {
				return childPath
			}
			if found := walkNegotiatedPrivateField(child, childPath, forbidden, published); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := walkNegotiatedPrivateField(child, path+"[]", forbidden, published); found != "" {
				return found
			}
		}
	}
	return ""
}

func negotiatedFieldIsPublished(path string, value any, published map[string]func(any) bool) bool {
	admissible, declared := published[path]
	return declared && admissible(value)
}

// publishedNegotiatedTransitionTokens names every position where the status
// schemas publish a "token" property, and what a value there must look like.
// Both positions render argv: $defs/executable_transition_argument requires the
// token on an execute argument, and $defs/transition_argument permits it on the
// arguments of a collect input whose capture_operation this product performs.
// Nothing else in the payload may carry the key.
func publishedNegotiatedTransitionTokens() map[string]func(any) bool {
	return map[string]func(any) bool{
		"next_transition.execute.arguments[].token":          negotiatedArgvToken,
		"next_transition.collect.inputs[].arguments[].token": negotiatedArgvToken,
	}
}

// negotiatedArgvToken accepts only what the schema says a token is: a non-empty
// "--flag=value" argv string. A provider handle smuggled into this position
// under the published name still has to look like a flag to pass, which no
// store path, lock or directory does.
func negotiatedArgvToken(value any) bool {
	text, isString := value.(string)
	return isString && strings.HasPrefix(text, "--") && strings.Contains(text, "=")
}

// TestNegotiatedPrivacyGuardStillRejectsProviderPrivateFields is the proof that
// making the guard discriminate did not disarm it. The published token position
// is admitted; a genuine handle is still caught, including one wearing the
// published key's own name one level away from where the contract declares it.
func TestNegotiatedPrivacyGuardStillRejectsProviderPrivateFields(t *testing.T) {
	forbidden := map[string]struct{}{
		"repository": {}, "store_path": {}, "authority_path": {}, "receipt_path": {}, "lock": {}, "locks": {}, "token": {}, "tokens": {}, "directory": {},
	}
	published := publishedNegotiatedTransitionTokens()
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "published collect token is admitted",
			payload: `{"next_transition":{"kind":"collect","collect":{"inputs":[{"capture_operation":"review.capture-result","arguments":[{"name":"lineage","value":"review-abc","token":"--lineage=review-abc"}]}]}}}`,
		},
		{
			name:    "published execute token is admitted",
			payload: `{"next_transition":{"kind":"execute","execute":{"arguments":[{"name":"lineage","value":"review-abc","token":"--lineage=review-abc"}]}}}`,
		},
		{
			name:    "provider handle beside the published token is rejected",
			payload: `{"next_transition":{"kind":"collect","collect":{"inputs":[{"arguments":[{"name":"lineage","token":"--lineage=review-abc","store_path":"/home/u/.gentle-ai/review"}]}]}}}`,
			want:    "next_transition.collect.inputs[].arguments[].store_path",
		},
		{
			name:    "credential token outside the published position is rejected",
			payload: `{"authority":{"token":"ghp_realsecret"},"next_transition":{"kind":"stop"}}`,
			want:    "authority.token",
		},
		{
			name:    "credential token one level above the published position is rejected",
			payload: `{"next_transition":{"kind":"collect","collect":{"inputs":[{"token":"ghp_realsecret","arguments":[{"name":"lineage","token":"--lineage=review-abc"}]}]}}}`,
			want:    "next_transition.collect.inputs[].token",
		},
		{
			name:    "handle wearing the published name and position is still rejected",
			payload: `{"next_transition":{"kind":"execute","execute":{"arguments":[{"name":"lineage","token":"/home/u/.gentle-ai/review/authority.json"}]}}}`,
			want:    "next_transition.execute.arguments[].token",
		},
		{
			name:    "lock handle anywhere is rejected",
			payload: `{"next_transition":{"kind":"execute","execute":{"binding":{"lock":"/run/gentle-ai.lock"}}}}`,
			want:    "next_transition.execute.binding.lock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var document any
			if err := json.Unmarshal([]byte(tt.payload), &document); err != nil {
				t.Fatal(err)
			}
			if got := negotiatedPrivateFieldPath(document, forbidden, published); got != tt.want {
				t.Fatalf("negotiatedPrivateFieldPath = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestNegotiatedPrivacyGuardMatchesTheNameCheckWhenNothingIsPublished pins the
// relationship between the two guards: with an empty published set the
// discriminating walker reports a violation for exactly the payloads the
// original key-name check rejected, so the only behavioural difference is the
// declared positions.
func TestNegotiatedPrivacyGuardMatchesTheNameCheckWhenNothingIsPublished(t *testing.T) {
	forbidden := map[string]struct{}{"store_path": {}, "token": {}, "lock": {}}
	payloads := []string{
		`{"next_transition":{"kind":"collect","collect":{"inputs":[{"arguments":[{"name":"lineage","token":"--lineage=review-abc"}]}]}}}`,
		`{"next_transition":{"kind":"execute","execute":{"arguments":[{"name":"lineage","token":"--lineage=review-abc"}]}}}`,
		`{"authority":{"store_path":"/home/u/.gentle-ai"}}`,
		`{"next_transition":{"kind":"stop","reason_code":"native_stop_required"}}`,
	}
	for _, payload := range payloads {
		var document any
		if err := json.Unmarshal([]byte(payload), &document); err != nil {
			t.Fatal(err)
		}
		named := findCapabilityForbiddenField(document, forbidden)
		walked := negotiatedPrivateFieldPath(document, forbidden, map[string]func(any) bool{})
		if (named == "") != (walked == "") {
			t.Fatalf("payload %s: name check = %q, path check = %q", payload, named, walked)
		}
		if walked != "" && !strings.HasSuffix(walked, named) {
			t.Fatalf("payload %s: path check %q does not report the name-check field %q", payload, walked, named)
		}
	}
}
