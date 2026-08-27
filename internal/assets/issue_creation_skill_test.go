package assets

import (
	"regexp"
	"strings"
	"testing"
)

func TestIssueCreationSkillPublicationContract(t *testing.T) {
	content := MustRead("skills/issue-creation/SKILL.md")

	contracts := []struct {
		name  string
		terms []string
	}{
		{"proportional discovery", []string{"Fast path", "Minimal discovery", "missing or stale facts"}},
		{"exact target", []string{"[HOST/]OWNER/REPO", "Never assume the current repository", "TARGET=$HOST/$REPO"}},
		{"single format authority", []string{"YAML Issue Forms are the single format authority", "omit `markdown` guidance"}},
		{"current duplicate search", []string{"open-and-closed duplicate search", "Reuse that result while it remains current", "--state all"}},
		{"evidence-based duplicate handling", []string{"read from the target host", "Compare each candidate's body controls and required answers with the selected YAML form", "Unavailable body, target mismatch, incomplete data, or ambiguous classification is `unknown`", "Comment there instead", "repair it in place", "never auto-rewrite or approve"}},
		{"semantic form translation", []string{"declared order", "`input` / `textarea`", "`dropdown`", "`checkboxes`", "`validations.required`", "first-person", "textarea.attributes.render", "`attributes.multiple` selection mode", "`dropdown.attributes.multiple: true`", "otherwise treat it as single-select", "every dropdown selection to exactly match a declared option", "A required dropdown must have at least one valid selection", "preserve every valid reviewed selection in declared options order"}},
		{"private discovery, body, and read-back lifecycle", []string{"private temporary files outside repositories", "Do not print the contents of any protected file", "owner-only temporary directory", "`DISCOVERY_FILE`, `BODY_FILE`, plus `READBACK_FILE`", "`0700`/`0600`, or strict Windows ACL equivalents", "Clean up all three files on every"}},
		{"file-backed CLI publication", []string{"gh issue create", "--body-file \"$BODY_FILE\"", "gh issue comment"}},
		{"private body-bearing read-back", []string{"read it back from that host into `READBACK_FILE`", "Redirect stdout from both body-bearing read-back commands", "Validate and compare only from `READBACK_FILE`"}},
		{"bounded outcomes", []string{"confirmed | no_write | unknown", "one create or comment attempt with no blind retry", "stop all mutations and retries"}},
		{"target-host verification", []string{"target-host read-back", "CRLF-to-LF", "trailing-final-newline normalization"}},
		{"label policy", []string{"labels declared by the selected form", "permitted for the actor", "Never add `status:approved`"}},
		{"comment parent identity", []string{"returned comment's `issue_url`", "issue `$NUMBER` in `$REPO` on `$HOST`", "absent or mismatched parent identity is `unknown`", "Clean up and stop all mutations and retries"}},
		{"candidate target identity", []string{"returned candidate number and URL in `DISCOVERY_FILE`", "`$CANDIDATE_NUMBER` in `$REPO` on `$HOST` before classification", "a mismatch is `unknown`"}},
		{"canonical version", []string{"version: \"1.3\""}},
	}

	for _, contract := range contracts {
		t.Run(contract.name, func(t *testing.T) {
			for _, term := range contract.terms {
				if !strings.Contains(content, term) {
					t.Errorf("issue-creation skill is missing %s contract marker %q", contract.name, term)
				}
			}
		})
	}

	forbidden := []string{
		"--web",
		"gh browse",
		"POST /repos/",
		"API_BASE",
		"PAYLOAD_FILE",
		"http.Client",
		"curl ",
		"hosted publisher",
		"Go publisher",
		"Markdown template",
		`--body "$BODY"`,
		`${LABEL_ARGS[@]}`,
	}
	for _, term := range forbidden {
		if strings.Contains(content, term) {
			t.Errorf("issue-creation skill contains forbidden alternate route %q", term)
		}
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	executionStart := strings.Index(normalized, "## Execution Steps\n")
	executionEnd := strings.Index(normalized, "## Output Contract\n")
	if executionStart == -1 || executionEnd == -1 || executionStart >= executionEnd {
		t.Fatal("issue-creation skill must contain a concrete Execution Steps section before its Output Contract")
	}
	executionSteps := normalized[executionStart:executionEnd]
	commands := fencedBashCommands(t, executionSteps)
	expectedCommands := []string{
		`gh api --hostname "$HOST" --paginate "repos/$REPO/labels?per_page=100" --jq '.[].name'`,
		`gh issue list --repo "$TARGET" --state all --search "$QUERY" --limit 1000`,
		`gh issue view "$CANDIDATE_NUMBER" --repo "$TARGET" --json number,url,title,body >"$DISCOVERY_FILE"`,
		`gh issue create --repo "$TARGET" --title "$TITLE" --body-file "$BODY_FILE"`,
		`gh issue comment "$NUMBER" --repo "$TARGET" --body-file "$BODY_FILE"`,
		`gh issue view "$NUMBER" --repo "$TARGET" --json number,url,title,body,labels >"$READBACK_FILE"`,
		`gh api --hostname "$HOST" "repos/$REPO/issues/comments/$COMMENT_ID" >"$READBACK_FILE"`,
	}
	if strings.Join(commands, "\n") != strings.Join(expectedCommands, "\n") {
		t.Errorf("issue-creation skill fenced Bash commands changed:\n got: %q\nwant: %q", commands, expectedCommands)
	}

	createCommand := expectedCommands[3]
	commentCommand := expectedCommands[4]
	targetIndex := strings.Index(executionSteps, "derive and verify `HOST`, `REPO=OWNER/REPO`, and `TARGET=$HOST/$REPO`")
	discoveryIndex := strings.Index(executionSteps, "Authenticate to `HOST`; discover only missing")
	selectedFormIndex := strings.Index(executionSteps, "Select the one YAML form whose declared purpose matches")
	duplicateSearchIndex := strings.Index(executionSteps, expectedCommands[1])
	classificationIndex := strings.Index(executionSteps, "Compare each candidate's body controls and required answers with the selected YAML form")
	materializationIndex := strings.Index(executionSteps, "Only when classification selects the new-issue path, process reviewed answers and materialize the body")
	for _, publication := range []struct {
		name  string
		index int
	}{
		{"create", strings.Index(executionSteps, createCommand)},
		{"comment", strings.Index(executionSteps, commentCommand)},
	} {
		if targetIndex == -1 || discoveryIndex == -1 || selectedFormIndex == -1 || duplicateSearchIndex == -1 || classificationIndex == -1 || materializationIndex == -1 || publication.index == -1 || targetIndex > discoveryIndex || discoveryIndex > selectedFormIndex || selectedFormIndex > duplicateSearchIndex || duplicateSearchIndex > classificationIndex || classificationIndex > materializationIndex || materializationIndex > publication.index {
			t.Errorf("issue-creation skill Execution Steps must order target, discovery, selected form, duplicate search and classification, new-issue materialization, then %s publication", publication.name)
		}
	}

	labelContract := `   | Permitted labels | Command suffix |
   | --- | --- |
   | Zero | Append no ` + "`--label`" + ` tokens. |
   | Each permitted label | Append exactly one separate ` + "`--label \"$PERMITTED_LABEL\"`" + ` pair. |`
	if count := strings.Count(executionSteps, labelContract); count != 1 {
		t.Errorf("issue-creation skill must contain exactly one scoped zero- and multi-label expansion contract, found %d", count)
	}
}

func fencedBashCommands(t *testing.T, executionSteps string) []string {
	t.Helper()
	fences := regexp.MustCompile("(?ms)^[ \\t]*```bash\\n(.*?)^[ \\t]*```[ \\t]*$").FindAllStringSubmatch(executionSteps, -1)
	var commands []string
	for _, fence := range fences {
		for _, line := range strings.Split(fence[1], "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "gh ") {
				commands = append(commands, line)
			}
		}
	}
	return commands
}
