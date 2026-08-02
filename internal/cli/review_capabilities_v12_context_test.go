package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

func TestReviewCapabilitiesV13AdvertisesProviderAdmissionAndRecovery(t *testing.T) {
	surface := reviewCapabilitiesStaticSurface()
	for _, want := range []ReviewCapabilityFeature{
		{Name: "opaque_repository_context", Supported: true, Requires: []string{"compact_v2_authority", "native_next_transition"}},
		{Name: "provider_artifact_admission", Supported: true, Requires: []string{"compact_v2_authority", "native_frozen_candidate_context", "opaque_repository_context"}},
		{Name: "provider_targeted_validation_request", Supported: true, Requires: []string{"compact_v2_authority", "native_next_transition"}},
		{Name: "recovered_correction_evidence", Supported: true, Requires: []string{"compact_v2_authority", "provider_targeted_validation_request"}},
		{Name: "validating_result_reopen", Supported: true, Requires: []string{"compact_v2_authority", "provider_artifact_admission"}},
	} {
		if !slices.ContainsFunc(surface.Features.Optional, func(got ReviewCapabilityFeature) bool {
			return got.Name == want.Name && got.Supported == want.Supported && slices.Equal(got.Requires, want.Requires)
		}) {
			t.Fatalf("v1.3 optional capabilities missing %#v: %#v", want, surface.Features.Optional)
		}
	}
	for _, schema := range []string{
		reviewtransaction.ArtifactSubjectSchemaV1,
		reviewtransaction.AdmittedReviewerResultSchemaV1,
		reviewtransaction.TargetedValidationRequestSchema,
	} {
		if !slices.Contains(surface.Schemas, schema) {
			t.Fatalf("v1.3 schemas do not advertise %q: %v", schema, surface.Schemas)
		}
	}
}

func TestReviewCapabilitiesV10ThroughV12ArtifactsRemainByteIdentical(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "review-integration", "v1")
	want := map[string]string{
		"fixtures/capabilities.fixture.json":      "b3ca822189a236f2d891628c665ca23e308bf5185a1701e1f07231bd970461bb",
		"fixtures/capabilities-v1.1.fixture.json": "1b3dc40dce7bfb5d3ecc7e92af68d66e71b733ba0b0f71ba94d3c633adc48bcf",
		"fixtures/capabilities-v1.2.fixture.json": "2970d21cd95a7fcaea6547c47a591a5151046e7ede658b3e8c5b9a9c5d106b65",
		"schemas/capabilities.schema.json":        "ad333177494a251beac153f74bd751fa77126a9968aad69e64fc2abf15cff0f7",
		"schemas/capabilities-v1.1.schema.json":   "2b14162284f375f8563e49d3a28caaa0aabb572094d8d290eb61844b1353af78",
		"schemas/capabilities-v1.2.schema.json":   "df1722adcd9c999edbef090bfd5d9a9713f6852a9bc9cb79684ef7c9c91c0d62",
	}
	for name, expected := range want {
		payload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		if actual := hex.EncodeToString(digest[:]); actual != expected {
			t.Fatalf("%s digest = %s, want %s", name, actual, expected)
		}
	}
}
