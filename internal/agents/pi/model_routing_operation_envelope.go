package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ModelRoutingOperation identifies a model-routing protocol operation.
type ModelRoutingOperation string

const (
	ModelRoutingOperationValidate ModelRoutingOperation = "validate"
	ModelRoutingOperationApply    ModelRoutingOperation = "apply"
	ModelRoutingOperationInspect  ModelRoutingOperation = "inspect"
)

// modelRoutingOperationEnvelope contains the validated shared fields. Result is
// intentionally raw so each operation can validate its own result contract.
type modelRoutingOperationEnvelope struct {
	Version   int
	Contract  string
	Operation ModelRoutingOperation
	OK        bool
	ExitClass string
	Result    json.RawMessage
}

type modelRoutingOperationEnvelopeJSON struct {
	Version   *int                   `json:"version"`
	Contract  *string                `json:"contract"`
	Operation *ModelRoutingOperation `json:"operation"`
	OK        *bool                  `json:"ok"`
	ExitClass *string                `json:"exitClass"`
	Result    json.RawMessage        `json:"result"`
}

// ModelRoutingResponseErrorKind classifies a rejected operation response.
type ModelRoutingResponseErrorKind string

const (
	ModelRoutingResponseErrorMalformed            ModelRoutingResponseErrorKind = "malformed"
	ModelRoutingResponseErrorOversized            ModelRoutingResponseErrorKind = "oversized"
	ModelRoutingResponseErrorUnsupportedVersion   ModelRoutingResponseErrorKind = "unsupported-version"
	ModelRoutingResponseErrorUnsupportedContract  ModelRoutingResponseErrorKind = "unsupported-contract"
	ModelRoutingResponseErrorUnsupportedOperation ModelRoutingResponseErrorKind = "unsupported-operation"

	ModelRoutingResponseErrorContract  = ModelRoutingResponseErrorUnsupportedContract
	ModelRoutingResponseErrorOperation = ModelRoutingResponseErrorUnsupportedOperation
)

var (
	ErrModelRoutingResponseMalformed            = errors.New("malformed model-routing response")
	ErrModelRoutingResponseOversized            = errors.New("model-routing response is oversized")
	ErrModelRoutingResponseUnsupportedVersion   = errors.New("model-routing response uses an unsupported version")
	ErrModelRoutingResponseUnsupportedContract  = errors.New("model-routing response uses an unsupported contract")
	ErrModelRoutingResponseUnsupportedOperation = errors.New("model-routing response uses an unsupported operation")

	ErrModelRoutingResponseContract  = ErrModelRoutingResponseUnsupportedContract
	ErrModelRoutingResponseOperation = ErrModelRoutingResponseUnsupportedOperation
)

// ModelRoutingResponseError reports one deterministic operation response
// rejection. Its text is bounded to metadata; Cause remains available through
// errors.Is/errors.As without exposing provider result bytes.
type ModelRoutingResponseError struct {
	Kind              ModelRoutingResponseErrorKind
	Operation         ModelRoutingOperation
	ExpectedOperation ModelRoutingOperation
	Cause             error
}

func (e *ModelRoutingResponseError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Operation != "" && e.ExpectedOperation != "" && e.Operation != e.ExpectedOperation {
		return fmt.Sprintf("Pi model-routing response error (%s, operation %q, expected %q)", e.Kind, e.Operation, e.ExpectedOperation)
	}
	return fmt.Sprintf("Pi model-routing response error (%s, operation %q)", e.Kind, e.Operation)
}

func (e *ModelRoutingResponseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// decodeModelRoutingOperationEnvelope validates only the shared operation
// envelope. Result remains raw for operation-specific decoding by its caller.
func decodeModelRoutingOperationEnvelope(payload []byte, expectedOperation ModelRoutingOperation) (modelRoutingOperationEnvelope, error) {
	operation := expectedOperation
	malformed := func(cause error) (modelRoutingOperationEnvelope, error) {
		return modelRoutingOperationEnvelope{}, modelRoutingResponseError(ModelRoutingResponseErrorMalformed, operation, expectedOperation, ErrModelRoutingResponseMalformed, cause)
	}
	if len(payload) > MaxModelRoutingResponseBytes {
		return modelRoutingOperationEnvelope{}, modelRoutingResponseError(ModelRoutingResponseErrorOversized, operation, expectedOperation, ErrModelRoutingResponseOversized, nil)
	}

	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return malformed(errors.New("response must be one JSON object"))
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if err := scanJSONValue(decoder, otherObject); err != nil {
		return malformed(err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("response has trailing JSON")
		}
		return malformed(err)
	}

	decoder = json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var raw modelRoutingOperationEnvelopeJSON
	if err := decoder.Decode(&raw); err != nil {
		return malformed(err)
	}
	if raw.Operation != nil {
		operation = *raw.Operation
	}
	if raw.Version == nil {
		return malformed(errors.New("version is required"))
	}
	if *raw.Version != modelRoutingVersion {
		return modelRoutingOperationEnvelope{}, modelRoutingResponseError(ModelRoutingResponseErrorUnsupportedVersion, operation, expectedOperation, ErrModelRoutingResponseUnsupportedVersion, fmt.Errorf("got version %d", *raw.Version))
	}
	if raw.Contract == nil {
		return malformed(errors.New("contract is required"))
	}
	if *raw.Contract != modelRoutingContract {
		return modelRoutingOperationEnvelope{}, modelRoutingResponseError(ModelRoutingResponseErrorUnsupportedContract, operation, expectedOperation, ErrModelRoutingResponseUnsupportedContract, fmt.Errorf("got contract %q", *raw.Contract))
	}
	if raw.Operation == nil {
		return malformed(errors.New("operation is required"))
	}
	if operation != expectedOperation {
		return modelRoutingOperationEnvelope{}, modelRoutingResponseError(ModelRoutingResponseErrorUnsupportedOperation, operation, expectedOperation, ErrModelRoutingResponseUnsupportedOperation, fmt.Errorf("got operation %q", operation))
	}
	if raw.OK == nil {
		return malformed(errors.New("ok is required"))
	}
	if raw.ExitClass == nil {
		return malformed(errors.New("exitClass is required"))
	}
	if len(bytes.TrimSpace(raw.Result)) == 0 || modelRoutingNull(raw.Result) {
		return malformed(errors.New("result is required"))
	}

	return modelRoutingOperationEnvelope{
		Version:   *raw.Version,
		Contract:  *raw.Contract,
		Operation: operation,
		OK:        *raw.OK,
		ExitClass: *raw.ExitClass,
		Result:    append(json.RawMessage(nil), raw.Result...),
	}, nil
}

func modelRoutingResponseError(kind ModelRoutingResponseErrorKind, operation, expectedOperation ModelRoutingOperation, sentinel, cause error) error {
	return &ModelRoutingResponseError{
		Kind:              kind,
		Operation:         operation,
		ExpectedOperation: expectedOperation,
		Cause:             errors.Join(sentinel, cause),
	}
}
