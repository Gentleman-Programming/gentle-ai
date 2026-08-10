package cli

import "strings"

// sddAttemptContentDigestFlags names the sdd-attempt flags whose value is a
// caller-computed SHA-256 content digest, which is the only revision kind the
// CLI boundary may respell (#2523). The other revision-shaped flags stay
// byte-for-byte: --expected-revision, --expected-binding-revision, and
// --token carry ledger-issued identities the caller copies from product
// output, so a non-canonical spelling of any of them is a transcription
// fault the strict ledger contract (#2395) must keep refusing.
var sddAttemptContentDigestFlags = map[string]bool{
	"evidence-revision":            true,
	"remediates-evidence-revision": true,
}

// CanonicalizeSDDAttemptRevisionArgs rewrites, at argument-parsing time, the
// value of every content-digest sdd-attempt flag to the canonical
// sha256:<64-lowercase-hex> form when the value is one of the two unambiguous
// SHA-256 spellings: 64 hexadecimal digits with the sha256: prefix, or 64
// bare hexadecimal digits. Hex is case-insensitive by definition, so both
// spellings name the same digest the canonical form names; PowerShell's
// Get-FileHash prints the bare uppercase one by default (#2294). Every other
// value passes through byte-for-byte and refuses downstream exactly as
// before. The ledger stays a pure validator with no normalization of its own.
func CanonicalizeSDDAttemptRevisionArgs(args []string) []string {
	canonical := append([]string(nil), args...)
	for index := 0; index < len(canonical); index++ {
		argument := canonical[index]
		if !strings.HasPrefix(argument, "--") {
			continue
		}
		name := strings.TrimPrefix(argument, "--")
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			value := name[separator+1:]
			name = name[:separator]
			if sddAttemptContentDigestFlags[name] {
				canonical[index] = "--" + name + "=" + canonicalSHA256Revision(value)
			}
			continue
		}
		// The flag package consumes the next argument as this flag's value,
		// and validateSDDAttemptOperationFlags scans argv the same way.
		if sddAttemptContentDigestFlags[name] && index+1 < len(canonical) {
			canonical[index+1] = canonicalSHA256Revision(canonical[index+1])
			index++
		}
	}
	return canonical
}

// canonicalSHA256Revision maps the two unambiguous SHA-256 spellings to
// sha256:<64-lowercase-hex> and returns every other value unchanged, so the
// downstream refusal for ambiguous input is exactly the one shipped today.
// Surrounding whitespace is ignored for recognition because runSDDAttempt
// already trims these values before validation.
func canonicalSHA256Revision(value string) string {
	digest := strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
	if len(digest) != 64 {
		return value
	}
	for _, character := range digest {
		hexadecimal := (character >= '0' && character <= '9') ||
			(character >= 'a' && character <= 'f') ||
			(character >= 'A' && character <= 'F')
		if !hexadecimal {
			return value
		}
	}
	return "sha256:" + strings.ToLower(digest)
}
