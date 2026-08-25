package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	modelRoutingContract         = "gentle-pi.model-routing/v1"
	modelRoutingVersion          = 1
	modelRoutingCapabilities     = "capabilities"
	MaxCapabilitiesResponseBytes = 64 << 10
	maxCapabilitiesResponseBytes = MaxCapabilitiesResponseBytes
)

// Capabilities is the validated capability set reported by gentle-pi-models.
type Capabilities struct {
	Contract   string
	Supported  bool
	Operations []string
}

// CapabilitiesResponse is retained as a descriptive alias for callers that
// name the parsed provider response rather than the capability set.
type CapabilitiesResponse = Capabilities

// CapabilitiesErrorKind classifies a rejected capabilities response.
type CapabilitiesErrorKind string

const (
	CapabilitiesErrorMalformed             CapabilitiesErrorKind = "malformed"
	CapabilitiesErrorOversized             CapabilitiesErrorKind = "oversized"
	CapabilitiesErrorUnsupportedVersion    CapabilitiesErrorKind = "unsupported-version"
	CapabilitiesErrorUnsupportedContract   CapabilitiesErrorKind = "unsupported-contract"
	CapabilitiesErrorUnsupportedOperation  CapabilitiesErrorKind = "unsupported-operation"
	CapabilitiesErrorExplicitRemoteFailure CapabilitiesErrorKind = "explicit-remote-failure"
	CapabilitiesErrorInvalidOperations     CapabilitiesErrorKind = "invalid-operations"
)

var (
	ErrCapabilitiesMalformed             = errors.New("malformed capabilities response")
	ErrCapabilitiesOversized             = errors.New("capabilities response is oversized")
	ErrCapabilitiesUnsupportedVersion    = errors.New("capabilities response uses an unsupported version")
	ErrCapabilitiesUnsupportedContract   = errors.New("capabilities response uses an unsupported contract")
	ErrCapabilitiesUnsupportedOperation  = errors.New("capabilities response uses an unsupported operation")
	ErrCapabilitiesExplicitRemoteFailure = errors.New("capabilities provider reported failure")
	ErrCapabilitiesInvalidOperations     = errors.New("capabilities response has invalid operations")
)

// CapabilitiesError reports one deterministic capabilities-parser rejection.
type CapabilitiesError struct {
	Kind  CapabilitiesErrorKind
	Cause error
}

func (e *CapabilitiesError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("Pi capabilities error (%s)", e.Kind)
	}
	return fmt.Sprintf("Pi capabilities error (%s): %v", e.Kind, e.Cause)
}

func (e *CapabilitiesError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ParseCapabilitiesResponse validates one complete gentle-pi.model-routing/v1
// capabilities response without touching the host or invoking the provider.
func ParseCapabilitiesResponse(payload []byte) (Capabilities, error) {
	if len(payload) > MaxCapabilitiesResponseBytes {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorOversized, ErrCapabilitiesOversized)
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := scanJSONValue(decoder, otherObject); err != nil {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorMalformed, errors.Join(ErrCapabilitiesMalformed, err))
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("capabilities response contains a trailing JSON value")
		}
		return Capabilities{}, capabilitiesError(CapabilitiesErrorMalformed, errors.Join(ErrCapabilitiesMalformed, err))
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil || document == nil {
		if err == nil {
			err = errors.New("capabilities response must be one JSON object")
		}
		return Capabilities{}, capabilitiesError(CapabilitiesErrorMalformed, errors.Join(ErrCapabilitiesMalformed, err))
	}
	if hasTopLevelAlternativeOperationRepresentation(document) {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorInvalidOperations, ErrCapabilitiesInvalidOperations)
	}

	var envelope capabilitiesEnvelope
	decoder = json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorMalformed, errors.Join(ErrCapabilitiesMalformed, err))
	}
	if envelope.Version == nil {
		return Capabilities{}, malformedCapabilities("version is required")
	}
	if *envelope.Version != modelRoutingVersion {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorUnsupportedVersion, errors.Join(ErrCapabilitiesUnsupportedVersion, fmt.Errorf("got version %d", *envelope.Version)))
	}
	if envelope.Contract == nil {
		return Capabilities{}, malformedCapabilities("contract is required")
	}
	if *envelope.Contract != modelRoutingContract {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorUnsupportedContract, errors.Join(ErrCapabilitiesUnsupportedContract, fmt.Errorf("got contract %q", *envelope.Contract)))
	}
	if envelope.Operation == nil {
		return Capabilities{}, malformedCapabilities("operation is required")
	}
	if *envelope.Operation != modelRoutingCapabilities {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorUnsupportedOperation, errors.Join(ErrCapabilitiesUnsupportedOperation, fmt.Errorf("got operation %q", *envelope.Operation)))
	}
	if envelope.OK == nil {
		return Capabilities{}, malformedCapabilities("ok is required")
	}
	if envelope.ExitClass == nil {
		return Capabilities{}, malformedCapabilities("exitClass is required")
	}
	if !*envelope.OK || *envelope.ExitClass != "success" {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorExplicitRemoteFailure, ErrCapabilitiesExplicitRemoteFailure)
	}
	if len(envelope.Result) == 0 {
		return Capabilities{}, malformedCapabilities("result is required")
	}

	var resultDocument map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Result, &resultDocument); err != nil || resultDocument == nil {
		if err == nil {
			err = errors.New("result must be one JSON object")
		}
		return Capabilities{}, capabilitiesError(CapabilitiesErrorMalformed, errors.Join(ErrCapabilitiesMalformed, err))
	}
	if hasNestedAlternativeOperationRepresentation(resultDocument) {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorInvalidOperations, ErrCapabilitiesInvalidOperations)
	}

	var result capabilitiesResult
	decoder = json.NewDecoder(bytes.NewReader(envelope.Result))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		if _, present := resultDocument["operations"]; present {
			return Capabilities{}, capabilitiesError(CapabilitiesErrorInvalidOperations, errors.Join(ErrCapabilitiesInvalidOperations, err))
		}
		return Capabilities{}, capabilitiesError(CapabilitiesErrorMalformed, errors.Join(ErrCapabilitiesMalformed, err))
	}
	if result.Contract == nil || result.Supported == nil {
		return Capabilities{}, malformedCapabilities("result contract and supported are required")
	}
	if *result.Contract != modelRoutingContract {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorUnsupportedContract, errors.Join(ErrCapabilitiesUnsupportedContract, fmt.Errorf("result contract %q", *result.Contract)))
	}
	if !*result.Supported {
		return Capabilities{}, capabilitiesError(CapabilitiesErrorUnsupportedContract, ErrCapabilitiesUnsupportedContract)
	}
	if err := validateCapabilitiesOperations(result.Operations, resultDocument); err != nil {
		return Capabilities{}, err
	}

	return Capabilities{Contract: *result.Contract, Supported: *result.Supported, Operations: append([]string(nil), result.Operations...)}, nil
}

type capabilitiesEnvelope struct {
	Version   *int            `json:"version"`
	Contract  *string         `json:"contract"`
	Operation *string         `json:"operation"`
	OK        *bool           `json:"ok"`
	ExitClass *string         `json:"exitClass"`
	Result    json.RawMessage `json:"result"`
}

type capabilitiesResult struct {
	Contract   *string  `json:"contract"`
	Supported  *bool    `json:"supported"`
	Operations []string `json:"operations"`
}

func validateCapabilitiesOperations(operations []string, document map[string]json.RawMessage) error {
	if _, present := document["operations"]; !present || len(operations) != 4 {
		return capabilitiesError(CapabilitiesErrorInvalidOperations, ErrCapabilitiesInvalidOperations)
	}
	seen := make(map[string]bool, len(operations))
	for _, operation := range operations {
		if seen[operation] || !requiredCapabilitiesOperation(operation) {
			return capabilitiesError(CapabilitiesErrorInvalidOperations, ErrCapabilitiesInvalidOperations)
		}
		seen[operation] = true
	}
	for _, operation := range []string{"capabilities", "inspect", "validate", "apply"} {
		if !seen[operation] {
			return capabilitiesError(CapabilitiesErrorInvalidOperations, ErrCapabilitiesInvalidOperations)
		}
	}
	return nil
}

func requiredCapabilitiesOperation(operation string) bool {
	switch operation {
	case "capabilities", "inspect", "validate", "apply":
		return true
	default:
		return false
	}
}

func hasTopLevelAlternativeOperationRepresentation(document map[string]json.RawMessage) bool {
	for _, key := range []string{"operations", "capabilities", "supportedOperations", "supported_operations"} {
		if _, present := document[key]; present {
			return true
		}
	}
	return false
}

func hasNestedAlternativeOperationRepresentation(document map[string]json.RawMessage) bool {
	for _, key := range []string{"capabilities", "supportedOperations", "supported_operations"} {
		if _, present := document[key]; present {
			return true
		}
	}
	return false
}

func capabilitiesError(kind CapabilitiesErrorKind, cause error) error {
	return &CapabilitiesError{Kind: kind, Cause: cause}
}

func malformedCapabilities(reason string) error {
	return capabilitiesError(CapabilitiesErrorMalformed, fmt.Errorf("%w: %s", ErrCapabilitiesMalformed, reason))
}
