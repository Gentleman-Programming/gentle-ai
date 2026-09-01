package assets

import (
	"strings"
	"testing"
)

func TestIssueCreationSkillHasSanitizationRule(t *testing.T) {
	content := MustRead("skills/issue-creation/SKILL.md")

	for _, term := range []string{
		"practical privacy scan",
		"Immediately before mutation",
		"<project-name>",
		"<user>",
		"<hostname>",
		"<token>",
		"intentionally public identifiers",
		"useful reproduction structure",
	} {
		if !strings.Contains(content, term) {
			t.Errorf("issue-creation skill is missing privacy contract marker %q (see issue #1906)", term)
		}
	}

	privacyIndex := strings.Index(content, "Immediately before mutation")
	publicationCommands := []string{
		`gh issue create --repo "$TARGET" --title "$TITLE" --body-file "$BODY_FILE"`,
		`gh issue comment "$NUMBER" --repo "$TARGET" --body-file "$BODY_FILE"`,
	}
	for _, command := range publicationCommands {
		commandIndex := strings.Index(content, command)
		if privacyIndex == -1 || commandIndex == -1 || privacyIndex > commandIndex {
			t.Errorf("issue-creation skill must place its privacy scan before publication command %q", command)
		}
	}

	if !strings.Contains(content, `version: "1.4"`) {
		t.Errorf("issue-creation skill must preserve canonical version 1.4")
	}
}
