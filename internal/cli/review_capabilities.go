package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const ReviewIntegrationContractV1 = "gentle-ai.review-integration/v1"
const ReviewIntegrationCapabilitiesSchema = "gentle-ai.review-integration.capabilities/v1"
const ReviewIntegrationCapabilitiesSchemaID = "https://gentle-ai.dev/contracts/review-integration/v1/schemas/capabilities.schema.json"

const (
	reviewRefuterSchemaID   = "https://gentle-ai.dev/schema/review/refuter/v1"
	reviewReviewerSchemaID  = "https://gentle-ai.dev/schema/review/reviewer/v1"
	reviewValidatorSchemaID = "https://gentle-ai.dev/schema/review/validator/v1"
)

var reviewCapabilitiesBuildInfoReader = debug.ReadBuildInfo
var reviewCapabilitiesExecutablePath = os.Executable

type ReviewCapabilitiesResult struct {
	Schema        string                          `json:"schema"`
	Contract      string                          `json:"contract"`
	Protocol      ReviewCapabilitiesProtocol      `json:"protocol"`
	Package       ReviewCapabilitiesPackage       `json:"package"`
	Build         ReviewCapabilitiesBuild         `json:"build"`
	Executable    ReviewCapabilitiesExecutable    `json:"executable"`
	Operations    []string                        `json:"operations"`
	Gates         []string                        `json:"gates"`
	Projections   []string                        `json:"projections"`
	Schemas       []string                        `json:"schemas"`
	Features      ReviewCapabilitiesFeatures      `json:"features"`
	Compatibility ReviewCapabilitiesCompatibility `json:"compatibility"`
}

type ReviewCapabilitiesProtocol struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

type ReviewCapabilitiesPackage struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	ReleaseChannel string `json:"release_channel"`
}

type ReviewCapabilitiesBuild struct {
	ID            string `json:"id"`
	GoVersion     string `json:"go_version"`
	ModuleVersion string `json:"module_version"`
	VCS           string `json:"vcs"`
	VCSRevision   string `json:"vcs_revision"`
	VCSTime       string `json:"vcs_time"`
	VCSModified   string `json:"vcs_modified"`
}

type ReviewCapabilitiesExecutable struct {
	SHA256       string `json:"sha256"`
	Evidence     string `json:"evidence"`
	Verification string `json:"verification"`
}

type ReviewCapabilityFeature struct {
	Name      string   `json:"name"`
	Supported bool     `json:"supported"`
	Requires  []string `json:"requires"`
}

type ReviewCapabilitiesFeatures struct {
	Mandatory []ReviewCapabilityFeature `json:"mandatory"`
	Optional  []ReviewCapabilityFeature `json:"optional"`
}

type ReviewCapabilitiesCompatibility struct {
	MinimumProtocolMajor int                            `json:"minimum_protocol_major"`
	MaximumProtocolMajor int                            `json:"maximum_protocol_major"`
	AdditiveMinorPolicy  string                         `json:"additive_minor_policy"`
	UnknownMandatory     string                         `json:"unknown_mandatory"`
	UnknownOptional      string                         `json:"unknown_optional"`
	Modes                []string                       `json:"modes"`
	LegacyWindow         ReviewCapabilitiesLegacyWindow `json:"legacy_window"`
}

type ReviewCapabilitiesLegacyWindow struct {
	Mode                         string `json:"mode"`
	State                        string `json:"state"`
	ReadOnly                     bool   `json:"read_only"`
	DeprecationStarted           bool   `json:"deprecation_started"`
	Removal                      string `json:"removal"`
	MinimumCompatibilityReleases int    `json:"minimum_compatibility_releases"`
}

func RunReviewCapabilities(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review capabilities", stdout, "Report the repository-independent negotiated review provider surface and self-reported executable identity.")
	contract := flags.String("contract", ReviewIntegrationContractV1, "review integration contract to negotiate")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return reviewPreflightError(fmt.Errorf("unexpected review capabilities argument %q", flags.Arg(0)))
	}
	if err := validateReviewIntegrationContract(*contract); err != nil {
		return err
	}
	result, err := buildReviewCapabilities()
	if err != nil {
		return err
	}
	return encodeReviewJSON(stdout, result)
}

func validateReviewIntegrationContract(contract string) error {
	if contract != ReviewIntegrationContractV1 {
		return fmt.Errorf("unsupported review integration contract %q; supported contract is %s", contract, ReviewIntegrationContractV1)
	}
	return nil
}

func buildReviewCapabilities() (ReviewCapabilitiesResult, error) {
	version := strings.TrimSpace(AppVersion)
	if version == "" {
		return ReviewCapabilitiesResult{}, errors.New("gentle-ai package version is unavailable")
	}
	build, err := reviewCapabilitiesBuildIdentity(version)
	if err != nil {
		return ReviewCapabilitiesResult{}, err
	}
	executableDigest, err := reviewCapabilitiesExecutableDigest()
	if err != nil {
		return ReviewCapabilitiesResult{}, err
	}
	result := reviewCapabilitiesStaticSurface()
	result.Package = ReviewCapabilitiesPackage{Name: "gentle-ai", Version: version, ReleaseChannel: reviewReleaseChannel(version)}
	result.Build = build
	result.Executable = ReviewCapabilitiesExecutable{
		SHA256: executableDigest, Evidence: "self-reported", Verification: "compare-with-published-manifest",
	}
	if err := result.Validate(); err != nil {
		return ReviewCapabilitiesResult{}, fmt.Errorf("validate review capabilities: %w", err)
	}
	return result, nil
}

func reviewCapabilitiesStaticSurface() ReviewCapabilitiesResult {
	return ReviewCapabilitiesResult{
		Schema:   ReviewIntegrationCapabilitiesSchema,
		Contract: ReviewIntegrationContractV1,
		Protocol: ReviewCapabilitiesProtocol{Major: 1, Minor: 0},
		Operations: []string{
			"review.bind_sdd", "review.capabilities", "review.finalize", "review.start", "review.status", "review.validate",
		},
		Gates: []string{
			string(reviewtransaction.GatePostApply), string(reviewtransaction.GatePreCommit), string(reviewtransaction.GatePrePush),
			string(reviewtransaction.GatePrePR), string(reviewtransaction.GateRelease),
		},
		Projections: []string{string(reviewtransaction.ProjectionStaged), string(reviewtransaction.ProjectionWorkspace)},
		Schemas: []string{
			reviewtransaction.ReviewAuthorityStatusSchema,
			reviewtransaction.GateRequestSchema,
			ReviewIntegrationCapabilitiesSchema,
			ReviewIntegrationFailureSchema,
			ReviewIntegrationOperationSchema,
			ReviewIntegrationProjectionSchema,
			ReviewIntegrationStartSchema,
			ReviewIntegrationStatusSchema,
			reviewtransaction.ReceiptSchema,
			reviewtransaction.CompactReceiptSchema,
			reviewRefuterSchemaID,
			reviewReviewerSchemaID,
			reviewValidatorSchemaID,
		},
		Features: ReviewCapabilitiesFeatures{
			Mandatory: []ReviewCapabilityFeature{
				{Name: "compact_v2_authority", Supported: true, Requires: []string{}},
				{Name: "exact_receipt_replay", Supported: true, Requires: []string{"compact_v2_authority"}},
				{Name: "five_delivery_gates", Supported: true, Requires: []string{"compact_v2_authority"}},
				{Name: "immutable_snapshot", Supported: true, Requires: []string{}},
				{Name: "legacy_v1_target_scoped_read_only", Supported: true, Requires: []string{"target_scoped_status"}},
				{Name: "repository_independent_capabilities", Supported: true, Requires: []string{}},
				{Name: "restart_safe_projection", Supported: true, Requires: []string{"target_scoped_status"}},
				{Name: "sdd_receipt_binding", Supported: true, Requires: []string{"compact_v2_authority"}},
				{Name: "target_scoped_status", Supported: true, Requires: []string{"repository_independent_capabilities"}},
				{Name: "uniform_failure_envelope", Supported: true, Requires: []string{"repository_independent_capabilities"}},
			},
			Optional: []ReviewCapabilityFeature{
				{Name: "risk_reasons", Supported: true, Requires: []string{"repository_independent_capabilities"}},
			},
		},
		Compatibility: ReviewCapabilitiesCompatibility{
			MinimumProtocolMajor: 1, MaximumProtocolMajor: 1,
			AdditiveMinorPolicy: "optional-fields-only", UnknownMandatory: "reject", UnknownOptional: "ignore",
			Modes: []string{"compact-v2", "legacy-v1"},
			LegacyWindow: ReviewCapabilitiesLegacyWindow{
				Mode: "legacy-v1", State: "active", ReadOnly: true, DeprecationStarted: true,
				Removal: "not-scheduled", MinimumCompatibilityReleases: 1,
			},
		},
	}
}

func reviewCapabilitiesBuildIdentity(packageVersion string) (ReviewCapabilitiesBuild, error) {
	build := ReviewCapabilitiesBuild{GoVersion: runtime.Version(), VCSModified: "unknown"}
	if info, ok := reviewCapabilitiesBuildInfoReader(); ok && info != nil {
		if strings.TrimSpace(info.GoVersion) != "" {
			build.GoVersion = info.GoVersion
		}
		build.ModuleVersion = info.Main.Version
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs":
				build.VCS = setting.Value
			case "vcs.revision":
				build.VCSRevision = setting.Value
			case "vcs.time":
				build.VCSTime = setting.Value
			case "vcs.modified":
				if setting.Value != "true" && setting.Value != "false" {
					return ReviewCapabilitiesBuild{}, fmt.Errorf("invalid vcs.modified build setting %q", setting.Value)
				}
				build.VCSModified = setting.Value
			}
		}
	}
	if strings.TrimSpace(build.GoVersion) == "" {
		return ReviewCapabilitiesBuild{}, errors.New("Go build version is unavailable")
	}
	build.ID = reviewCapabilitiesBuildDigest(packageVersion, build)
	return build, nil
}

func reviewCapabilitiesBuildDigest(packageVersion string, build ReviewCapabilitiesBuild) string {
	preimage := struct {
		PackageVersion string `json:"package_version"`
		GoVersion      string `json:"go_version"`
		ModuleVersion  string `json:"module_version"`
		VCS            string `json:"vcs"`
		VCSRevision    string `json:"vcs_revision"`
		VCSTime        string `json:"vcs_time"`
		VCSModified    string `json:"vcs_modified"`
	}{
		PackageVersion: packageVersion, GoVersion: build.GoVersion, ModuleVersion: build.ModuleVersion,
		VCS: build.VCS, VCSRevision: build.VCSRevision, VCSTime: build.VCSTime, VCSModified: build.VCSModified,
	}
	payload, _ := json.Marshal(preimage)
	sum := sha256.Sum256(append([]byte("gentle-ai.review-build-identity/v1\x00"), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func reviewCapabilitiesExecutableDigest() (string, error) {
	path, err := reviewCapabilitiesExecutablePath()
	if err != nil {
		return "", fmt.Errorf("resolve gentle-ai executable: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open gentle-ai executable: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash gentle-ai executable: %w", err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func reviewReleaseChannel(version string) string {
	if version == "dev" || strings.Contains(strings.ToLower(version), "devel") {
		return "development"
	}
	if strings.Contains(version, "-") {
		return "prerelease"
	}
	return "stable"
}

func (result ReviewCapabilitiesResult) Validate() error {
	static := reviewCapabilitiesStaticSurface()
	if result.Schema != static.Schema || result.Contract != static.Contract || result.Protocol != static.Protocol ||
		!reflect.DeepEqual(result.Operations, static.Operations) || !reflect.DeepEqual(result.Gates, static.Gates) ||
		!reflect.DeepEqual(result.Projections, static.Projections) || !reflect.DeepEqual(result.Schemas, static.Schemas) ||
		!reflect.DeepEqual(result.Features, static.Features) || !reflect.DeepEqual(result.Compatibility, static.Compatibility) {
		return errors.New("capability surface does not match the negotiated v1 contract")
	}
	if result.Package.Name != "gentle-ai" || strings.TrimSpace(result.Package.Version) == "" || result.Package.ReleaseChannel != reviewReleaseChannel(result.Package.Version) {
		return errors.New("capability package identity is invalid")
	}
	if strings.TrimSpace(result.Build.GoVersion) == "" || (result.Build.VCSModified != "true" && result.Build.VCSModified != "false" && result.Build.VCSModified != "unknown") ||
		result.Build.ID != reviewCapabilitiesBuildDigest(result.Package.Version, result.Build) {
		return errors.New("capability build identity is invalid")
	}
	if !validReviewCapabilitySHA256(result.Executable.SHA256) || result.Executable.Evidence != "self-reported" || result.Executable.Verification != "compare-with-published-manifest" {
		return errors.New("capability executable identity is invalid")
	}
	return validateReviewCapabilityDependencies(result.Features)
}

func validateReviewCapabilityDependencies(features ReviewCapabilitiesFeatures) error {
	known := make(map[string]struct{}, len(features.Mandatory)+len(features.Optional))
	for _, feature := range append(append([]ReviewCapabilityFeature{}, features.Mandatory...), features.Optional...) {
		if strings.TrimSpace(feature.Name) == "" || feature.Requires == nil {
			return errors.New("capability feature is incomplete")
		}
		if _, duplicate := known[feature.Name]; duplicate {
			return fmt.Errorf("duplicate capability feature %q", feature.Name)
		}
		known[feature.Name] = struct{}{}
	}
	for _, feature := range append(append([]ReviewCapabilityFeature{}, features.Mandatory...), features.Optional...) {
		for _, dependency := range feature.Requires {
			if _, ok := known[dependency]; !ok {
				return fmt.Errorf("capability feature %q requires unknown feature %q", feature.Name, dependency)
			}
		}
	}
	return nil
}

func validReviewCapabilitySHA256(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
