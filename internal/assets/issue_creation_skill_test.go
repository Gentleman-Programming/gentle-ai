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
		{"private discovery, body, and read-back lifecycle", []string{"private temporary files outside repositories", "Do not print the contents of any protected file", "owner-only temporary directory", "`DISCOVERY_FILE`, `BODY_FILE`, `READBACK_FILE`, `PRE_READ_FILE`, plus `POST_READ_FILE`", "`0700`/`0600`, or strict Windows ACL equivalents", "Clean up all five files on every"}},
		{"file-backed CLI publication", []string{"gh issue create", "--body-file \"$BODY_FILE\"", "gh issue comment"}},
		{"private body-bearing read-back", []string{"read it back from that host into `READBACK_FILE`", "Redirect stdout from both body-bearing read-back commands", "Validate and compare only from `READBACK_FILE`"}},
		{"bounded outcomes", []string{"confirmed | no_write | unknown", "one create or comment attempt with no blind retry", "stop all mutations and retries"}},
		{"target-host verification", []string{"target-host read-back", "CRLF-to-LF", "trailing-final-newline normalization"}},
		{"create-time label policy", []string{"Create-time labels are limited to labels declared by the selected form", "discovered to exist", "permitted for the actor"}},
		{"delegated workflow mutation", []string{"Before ANY post-publication workflow mutation", "current direct human instruction", "read `references/delegated-workflow-actions.md` completely and follow it"}},
		{"comment parent identity", []string{"returned comment's `issue_url`", "issue `$NUMBER` in `$REPO` on `$HOST`", "absent or mismatched parent identity is `unknown`", "Clean up and stop all mutations and retries"}},
		{"candidate target identity", []string{"returned candidate number and URL in `DISCOVERY_FILE`", "`$CANDIDATE_NUMBER` in `$REPO` on `$HOST` before classification", "a mismatch is `unknown`"}},
		{"canonical version", []string{"version: \"1.4\""}},
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
		`gh issue view "$NUMBER" --repo "$TARGET" --json number,url,title,body,state,labels >"$READBACK_FILE"`,
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

func TestIssueCreationSkillDelegatedWorkflowMutationContract(t *testing.T) {
	canonical := MustRead("skills/issue-creation/SKILL.md")
	for _, term := range []string{
		"Before ANY post-publication workflow mutation, read `references/delegated-workflow-actions.md` completely and follow it.",
		"It is normative, not optional background.",
	} {
		if !strings.Contains(canonical, term) {
			t.Errorf("issue-creation skill must mandate delegated workflow reference loading %q", term)
		}
	}
	reference := MustRead("skills/issue-creation/references/delegated-workflow-actions.md")
	for _, term := range []string{
		"Only apply or remove existing labels (including categorization), close, or reopen",
		"authenticated actor has target-host `viewerPermission` `MAINTAIN` or `ADMIN` immediately before mutation",
		"Before each delegated mutation, read the exact bound target from the target host into `PRE_READ_FILE`",
		"validate its returned state and complete labels before preserving them as the pre-state",
		"status:needs-review`, `status:needs-design`, and `status:needs-info`", "exact comma-separated subset",
		"preserving every unrelated pre-state label", "never use sequential remove/add attempts",
		"read back the same target from the target host into `POST_READ_FILE`",
		"`confirmed` requires exact target identity, the requested state or label delta, and every unrelated pre-state label preserved",
		"`no_write` requires both an authoritative target-host rejection proving no mutation was accepted and successful target-host post-readback whose complete target state equals pre-read exactly: same number, URL, open/closed state, and entire label set",
		"`unknown` includes ambiguity, partial readback, identity mismatch, unavailable post-read, lost response, or any difference at all in requested or unrelated state/labels, including missing labels, and stops all further mutations and retries",
		"`status:approved` is a strict special case",
		"Protected policy labels are `status:approved`, `size:exception`, and any repository-defined gate-override or authorization label.",
		"Before any generic `$LABEL` add/remove command, explicitly reject every protected label from the generic path.",
		"Ordinary label actions fail closed when classification is unknown.", "Adding or removing a protected label requires current direct instruction verified on the target host as binding exact target/action to a repository maintainer or repository-authorized approver, plus authenticated actor `viewerPermission` `MAINTAIN` or `ADMIN`.",
		"`size:exception` additionally requires documented over-budget rationale; rationale never replaces policy authority.", "A repository-defined gate-override or authorization label has no generic fallback; require repository-defined protected handling or stop.",
		"one bounded mutation attempt with no blind retry",
		"`TRIAGE` is explicitly insufficient for `status:approved`; an unverifiable instructing principal means no mutation.",
	} {
		if !strings.Contains(reference, term) {
			t.Errorf("delegated workflow reference is missing contract marker %q", term)
		}
	}
	commands := fencedBashCommands(t, reference)
	wantCommands := []string{
		`gh issue view "$NUMBER" --repo "$TARGET" --json number,url,state,labels >"$PRE_READ_FILE"`,
		`gh pr view "$NUMBER" --repo "$TARGET" --json number,url,state,labels >"$PRE_READ_FILE"`,
		`gh repo view "$TARGET" --json viewerPermission`,
		`gh issue edit "$NUMBER" --repo "$TARGET" --add-label "status:approved" --remove-label "$CONFLICTING_LABELS"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --add-label "status:approved" --remove-label "$CONFLICTING_LABELS"`,
		`gh issue edit "$NUMBER" --repo "$TARGET" --add-label "status:approved"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --add-label "status:approved"`,
		`gh issue edit "$NUMBER" --repo "$TARGET" --remove-label "status:approved"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --remove-label "status:approved"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --add-label "size:exception"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --remove-label "size:exception"`,
		`gh issue edit "$NUMBER" --repo "$TARGET" --add-label "$LABEL"`,
		`gh issue edit "$NUMBER" --repo "$TARGET" --remove-label "$LABEL"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --add-label "$LABEL"`,
		`gh pr edit "$NUMBER" --repo "$TARGET" --remove-label "$LABEL"`,
		`gh issue close "$NUMBER" --repo "$TARGET"`,
		`gh issue reopen "$NUMBER" --repo "$TARGET"`,
		`gh pr close "$NUMBER" --repo "$TARGET"`,
		`gh pr reopen "$NUMBER" --repo "$TARGET"`,
		`gh issue view "$NUMBER" --repo "$TARGET" --json number,url,state,labels >"$POST_READ_FILE"`,
		`gh pr view "$NUMBER" --repo "$TARGET" --json number,url,state,labels >"$POST_READ_FILE"`,
	}
	if strings.Join(commands, "\n") != strings.Join(wantCommands, "\n") {
		t.Errorf("delegated workflow reference fenced Bash commands changed:\n got: %q\nwant: %q", commands, wantCommands)
	}
	protectedAuthorityGate := strings.Index(reference, wantCommands[2])
	ordinaryGenericBlock := strings.Index(reference, "## Ordinary bounded action and read-back")
	genericGuard := strings.Index(reference, "Before any generic `$LABEL` add/remove command, explicitly reject every protected label from the generic path.")
	firstGenericCommand := strings.Index(reference, wantCommands[11])
	if genericGuard == -1 || firstGenericCommand == -1 || genericGuard > firstGenericCommand || protectedAuthorityGate == -1 || ordinaryGenericBlock == -1 || strings.Index(reference, wantCommands[3]) <= protectedAuthorityGate || strings.Index(reference, wantCommands[10]) >= ordinaryGenericBlock {
		t.Error("protected commands must follow their authority gate and remain independent of the guarded generic block")
	}
	if strings.Contains(reference, "Never add `status:approved`") {
		t.Error("delegated workflow reference retains the blanket status:approved prohibition")
	}
}

func TestIssueCreationSkillDelegationAuthorityBoundaries(t *testing.T) {
	content := MustRead("skills/issue-creation/SKILL.md")
	cases := []struct {
		name  string
		terms []string
	}{
		{
			name: "approval instruction is target-host verified",
			terms: []string{
				"For `status:approved`, require target-host evidence binding the direct instructing principal to approval authority as a repository maintainer or repository-authorized approver.",
			},
		},
		{
			name: "protected labels require policy authority",
			terms: []string{
				"Protected policy labels are `status:approved`, `size:exception`, and any repository-defined gate-override/authorization label.",
				"Adding or removing a protected label requires current direct target-host-verified maintainer/authorized-approver instruction for exact add/remove plus authenticated actor target-host `viewerPermission` `MAINTAIN` or `ADMIN`; `TRIAGE` never suffices.",
				"`size:exception` also needs a documented rationale; unknown gate labels stop.",
				"Reject inferred/model-authored authority; atomic `status:approved`, exactly one attempt/readback; fail closed.",
			},
		},
		{
			name: "triage is constrained to granted workflow actions",
			terms: []string{
				"Verify target-host capability",
				"`TRIAGE` permits only GitHub-granted existing-label and issue/PR close/reopen actions",
				"push/merge/label creation/deletion/administration",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, term := range tc.terms {
				if !strings.Contains(content, term) {
					t.Errorf("issue-creation skill is missing %s marker %q", tc.name, term)
				}
			}
		})
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
