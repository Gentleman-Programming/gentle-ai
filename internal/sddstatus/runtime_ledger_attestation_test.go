package sddstatus

import "testing"

// TestFinalVerifyReportAttests pins which admitted reports may stand as the
// final verification attestation. The not-applicable cases are the point: a
// report that recorded an absent obligation describes exactly the bytes it
// classified, so it attests only the candidate it names.
func TestFinalVerifyReportAttests(t *testing.T) {
	const revision = "sha256:" + "a"
	const settled = "0123456789abcdef0123456789abcdef01234567"
	const drifted = "fedcba9876543210fedcba9876543210fedcba98"

	passing := VerifyReportAdmission{Valid: true, Verdict: "pass", EvidenceRevision: revision}
	notApplicable := VerifyReportAdmission{
		Valid: true, Verdict: VerifyVerdictNotApplicable,
		EvidenceRevision: revision, CandidateDigest: settled,
	}

	tests := []struct {
		name      string
		admission VerifyReportAdmission
		want      bool
	}{
		{name: "an executed passing report attests", admission: passing, want: true},
		{name: "a not-applicable report attests the tree it names", admission: notApplicable, want: true},
		{
			name: "it does not attest a drifted candidate",
			admission: VerifyReportAdmission{
				Valid: true, Verdict: VerifyVerdictNotApplicable,
				EvidenceRevision: revision, CandidateDigest: drifted,
			},
		},
		{
			name: "it does not attest without naming a tree",
			admission: VerifyReportAdmission{
				Valid: true, Verdict: VerifyVerdictNotApplicable, EvidenceRevision: revision,
			},
		},
		{
			name:      "a refused report never attests",
			admission: VerifyReportAdmission{Verdict: VerifyVerdictNotApplicable, EvidenceRevision: revision, CandidateDigest: settled},
		},
		{
			name: "a stale evidence revision never attests",
			admission: VerifyReportAdmission{
				Valid: true, Verdict: VerifyVerdictNotApplicable,
				EvidenceRevision: "sha256:" + "b", CandidateDigest: settled,
			},
		},
		{
			name:      "a failing verdict never attests",
			admission: VerifyReportAdmission{Valid: true, Verdict: "fail", EvidenceRevision: revision},
		},
		{
			name:      "pass_with_warnings is not final verification evidence",
			admission: VerifyReportAdmission{Valid: true, Verdict: "pass_with_warnings", EvidenceRevision: revision},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := finalVerifyReportAttests(tt.admission, revision, settled); got != tt.want {
				t.Fatalf("finalVerifyReportAttests() = %v, want %v", got, tt.want)
			}
		})
	}
}
