package reviewtransaction

// CandidateRelation is the closed relation vocabulary used by compact gate verdicts.
type CandidateRelation string

type ShadowRelation = CandidateRelation

const (
	ShadowRelationExact                 CandidateRelation = "exact"
	ShadowRelationCompatibleBaseAdvance CandidateRelation = "compatible_base_advance"
	ShadowRelationProvableContraction   CandidateRelation = "provable_contraction"
	ShadowRelationChanged               CandidateRelation = "changed"
	ShadowRelationUnrelated             CandidateRelation = "unrelated"
	ShadowRelationAmbiguous             CandidateRelation = "ambiguous"
	ShadowRelationUnknown               CandidateRelation = "unknown"
)
