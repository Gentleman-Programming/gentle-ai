package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ModelRoutingInspectResponse is one complete gentle-pi.model-routing/v1 inspect response.
type ModelRoutingInspectResponse struct {
	Version   int                    `json:"version"`
	Contract  string                 `json:"contract"`
	Operation string                 `json:"operation"`
	OK        bool                   `json:"ok"`
	ExitClass string                 `json:"exitClass"`
	Result    ModelRoutingInspection `json:"result"`
}

// ModelRoutingInspectEnvelope is retained as a descriptive alias for callers
// that name the parsed provider response by its wire representation.
type ModelRoutingInspectEnvelope = ModelRoutingInspectResponse

// ModelRoutingInspectErrorKind classifies one rejected inspect response.
type ModelRoutingInspectErrorKind string

const (
	ModelRoutingInspectErrorMalformed            ModelRoutingInspectErrorKind = "malformed"
	ModelRoutingInspectErrorOversized            ModelRoutingInspectErrorKind = "oversized"
	ModelRoutingInspectErrorUnsupportedVersion   ModelRoutingInspectErrorKind = "unsupported-version"
	ModelRoutingInspectErrorUnsupportedContract  ModelRoutingInspectErrorKind = "unsupported-contract"
	ModelRoutingInspectErrorUnsupportedOperation ModelRoutingInspectErrorKind = "unsupported-operation"

	// Compatibility names for callers that use the shorter envelope labels.
	ModelRoutingInspectErrorContract  = ModelRoutingInspectErrorUnsupportedContract
	ModelRoutingInspectErrorOperation = ModelRoutingInspectErrorUnsupportedOperation
)

var (
	ErrModelRoutingInspectMalformed            = errors.New("malformed model-routing inspect response")
	ErrModelRoutingInspectOversized            = errors.New("model-routing inspect response is oversized")
	ErrModelRoutingInspectUnsupportedVersion   = errors.New("model-routing inspect response uses an unsupported version")
	ErrModelRoutingInspectUnsupportedContract  = errors.New("model-routing inspect response uses an unsupported contract")
	ErrModelRoutingInspectUnsupportedOperation = errors.New("model-routing inspect response uses an unsupported operation")

	// Compatibility sentinels for callers that use the shorter envelope labels.
	ErrModelRoutingInspectContract  = ErrModelRoutingInspectUnsupportedContract
	ErrModelRoutingInspectOperation = ErrModelRoutingInspectUnsupportedOperation
)

// ModelRoutingInspectError reports one deterministic inspect-parser rejection.
type ModelRoutingInspectError struct {
	Kind  ModelRoutingInspectErrorKind
	Cause error
}

func (e *ModelRoutingInspectError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("Pi model-routing inspect error (%s)", e.Kind)
	}
	return fmt.Sprintf("Pi model-routing inspect error (%s): %v", e.Kind, e.Cause)
}

func (e *ModelRoutingInspectError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ParseModelRoutingInspectResponse parses one strict outer inspect envelope and
// delegates the complete result contract to decodeModelRoutingInspection.
// Process exit status and ok/exitClass consistency are intentionally outside
// this parser's responsibility.
func ParseModelRoutingInspectResponse(payload []byte) (ModelRoutingInspectResponse, error) {
	if len(payload) > MaxModelRoutingResponseBytes {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorOversized, ErrModelRoutingInspectOversized, nil)
	}

	trimmed := bytes.TrimSpace(payload)
	if err := scanModelRoutingInspectResponse(trimmed); err != nil {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorMalformed, ErrModelRoutingInspectMalformed, err)
	}

	var envelope modelRoutingInspectEnvelope
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorMalformed, ErrModelRoutingInspectMalformed, err)
	}
	if envelope.Version == nil {
		return ModelRoutingInspectResponse{}, malformedModelRoutingInspect("version is required")
	}
	if *envelope.Version != modelRoutingVersion {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorUnsupportedVersion, ErrModelRoutingInspectUnsupportedVersion, fmt.Errorf("got version %d", *envelope.Version))
	}
	if envelope.Contract == nil {
		return ModelRoutingInspectResponse{}, malformedModelRoutingInspect("contract is required")
	}
	if *envelope.Contract != modelRoutingContract {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorUnsupportedContract, ErrModelRoutingInspectUnsupportedContract, fmt.Errorf("got contract %q", *envelope.Contract))
	}
	if envelope.Operation == nil {
		return ModelRoutingInspectResponse{}, malformedModelRoutingInspect("operation is required")
	}
	if *envelope.Operation != "inspect" {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorUnsupportedOperation, ErrModelRoutingInspectUnsupportedOperation, fmt.Errorf("got operation %q", *envelope.Operation))
	}
	if envelope.OK == nil {
		return ModelRoutingInspectResponse{}, malformedModelRoutingInspect("ok is required")
	}
	if envelope.ExitClass == nil {
		return ModelRoutingInspectResponse{}, malformedModelRoutingInspect("exitClass is required")
	}
	if len(bytes.TrimSpace(envelope.Result)) == 0 || modelRoutingNull(envelope.Result) {
		return ModelRoutingInspectResponse{}, malformedModelRoutingInspect("result is required")
	}

	result, err := decodeModelRoutingInspection(envelope.Result)
	if err != nil {
		return ModelRoutingInspectResponse{}, modelRoutingInspectError(ModelRoutingInspectErrorMalformed, ErrModelRoutingInspectMalformed, err)
	}
	return ModelRoutingInspectResponse{
		Version:   *envelope.Version,
		Contract:  *envelope.Contract,
		Operation: *envelope.Operation,
		OK:        *envelope.OK,
		ExitClass: *envelope.ExitClass,
		Result:    result,
	}, nil
}

type modelRoutingInspectEnvelope struct {
	Version   *int            `json:"version"`
	Contract  *string         `json:"contract"`
	Operation *string         `json:"operation"`
	OK        *bool           `json:"ok"`
	ExitClass *string         `json:"exitClass"`
	Result    json.RawMessage `json:"result"`
}

func scanModelRoutingInspectResponse(payload []byte) error {
	if len(payload) == 0 || payload[0] != '{' {
		return errors.New("model-routing inspect response must be one object")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := scanJSONValue(decoder, otherObject); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("model-routing inspect response has trailing JSON")
		}
		return err
	}
	return nil
}

func modelRoutingInspectError(kind ModelRoutingInspectErrorKind, sentinel, cause error) error {
	return &ModelRoutingInspectError{Kind: kind, Cause: errors.Join(sentinel, cause)}
}

func malformedModelRoutingInspect(reason string) error {
	return modelRoutingInspectError(ModelRoutingInspectErrorMalformed, ErrModelRoutingInspectMalformed, errors.New(reason))
}
