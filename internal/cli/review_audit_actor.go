package cli

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// reviewSelfRecoveryFallbackActor is the actor recorded in the CAS audit
// trail when no usable Git identity can be derived for a repository. It must
// never be empty and derivation must never fail the operation: refusing
// self-recovery because Git identity is unset or unsafe would reintroduce
// the exact deadlock class self-derived recovery exists to remove.
const reviewSelfRecoveryFallbackActor = "gentle-ai-self-recovery@localhost"

// reviewAuditActor derives the actor recorded for a self-derived recovery
// binding from the repository's Git identity. It never fails and never
// returns an empty string. The fallback chain is: "Name <email>" from
// user.name/user.email -> email alone -> GIT_AUTHOR_EMAIL ->
// GIT_COMMITTER_EMAIL -> a fixed literal actor. Any candidate value
// containing \r or \n is rejected and the chain falls through to the next
// candidate: the derived binding is LF-delimited
// (reviewTransitionRecoveryAuthorization and its siblings), so a newline
// embedded in a Git identity value would forge additional binding lines.
func reviewAuditActor(ctx context.Context, root string) string {
	name := reviewGitConfigValue(ctx, root, "user.name")
	email := reviewGitConfigValue(ctx, root, "user.email")
	if name != "" && email != "" && reviewAuditActorValueSafe(name) && reviewAuditActorValueSafe(email) {
		return name + " <" + email + ">"
	}
	if email != "" && reviewAuditActorValueSafe(email) {
		return email
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" && reviewAuditActorValueSafe(value) {
			return value
		}
	}
	return reviewSelfRecoveryFallbackActor
}

// reviewGitConfigValue reads one Git config value from the exact resolved
// repository root, never cwd-relative. It is read-only and swallows every
// error (missing key, non-repository root, git unavailable): the fallback
// chain above is the only place absence is meaningful.
func reviewGitConfigValue(ctx context.Context, root, key string) string {
	output, err := exec.CommandContext(ctx, "git", "-C", root, "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// reviewAuditActorValueSafe rejects any value containing \r or \n.
func reviewAuditActorValueSafe(value string) bool {
	return !strings.ContainsAny(value, "\r\n")
}
