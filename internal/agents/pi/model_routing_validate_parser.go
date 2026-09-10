package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ModelRoutingValidationResult is the typed result of a model-routing validate operation.
type ModelRoutingValidationResult struct {
	Contract    string                   `json:"contract"`
	OK          bool                     `json:"ok"`
	Diagnostics []ModelRoutingDiagnostic `json:"diagnostics"`
}

// ModelRoutingValidateResponse is one complete model-routing validate response.
type ModelRoutingValidateResponse struct {
	Version   int                          `json:"version"`
	Contract  string                       `json:"contract"`
	Operation ModelRoutingOperation        `json:"operation"`
	OK        bool                         `json:"ok"`
	ExitClass string                       `json:"exitClass"`
	Result    ModelRoutingValidationResult `json:"result"`
}

type modelRoutingValidationResultJSON struct {
	Contract    *string         `json:"contract"`
	OK          *bool           `json:"ok"`
	Diagnostics json.RawMessage `json:"diagnostics"`
}

// ParseModelRoutingValidateResponse parses one complete typed validate response.
// Envelope identity and bounds are owned by decodeModelRoutingOperationEnvelope;
// this parser only decodes and validates the operation-specific result.
func ParseModelRoutingValidateResponse(payload []byte) (ModelRoutingValidateResponse, error) {
	envelope, err := decodeModelRoutingOperationEnvelope(payload, ModelRoutingOperationValidate)
	if err != nil {
		return ModelRoutingValidateResponse{}, err
	}
	result, err := decodeModelRoutingValidationResult(envelope.Result)
	if err != nil {
		return ModelRoutingValidateResponse{}, modelRoutingResponseError(
			ModelRoutingResponseErrorMalformed,
			envelope.Operation,
			ModelRoutingOperationValidate,
			ErrModelRoutingResponseMalformed,
			err,
		)
	}
	return ModelRoutingValidateResponse{
		Version:   envelope.Version,
		Contract:  envelope.Contract,
		Operation: envelope.Operation,
		OK:        envelope.OK,
		ExitClass: envelope.ExitClass,
		Result:    result,
	}, nil
}

func decodeModelRoutingValidationResult(data []byte) (ModelRoutingValidationResult, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return ModelRoutingValidationResult{}, errors.New("model-routing validate result must be one object")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var raw modelRoutingValidationResultJSON
	if err := decoder.Decode(&raw); err != nil {
		return ModelRoutingValidationResult{}, err
	}
	if raw.Contract == nil {
		return ModelRoutingValidationResult{}, errors.New("model-routing validate result contract is required")
	}
	if *raw.Contract != modelRoutingContract {
		return ModelRoutingValidationResult{}, fmt.Errorf("invalid model-routing validate result contract %q", *raw.Contract)
	}
	if raw.OK == nil {
		return ModelRoutingValidationResult{}, errors.New("model-routing validate result ok is required")
	}
	if len(bytes.TrimSpace(raw.Diagnostics)) == 0 || modelRoutingNull(raw.Diagnostics) {
		return ModelRoutingValidationResult{}, errors.New("model-routing validate result diagnostics is required")
	}

	decoder = json.NewDecoder(bytes.NewReader(raw.Diagnostics))
	decoder.DisallowUnknownFields()
	var diagnostics []ModelRoutingDiagnostic
	if err := decoder.Decode(&diagnostics); err != nil {
		return ModelRoutingValidationResult{}, err
	}
	if err := validateModelRoutingDiagnostics(raw.Diagnostics, diagnostics); err != nil {
		return ModelRoutingValidationResult{}, err
	}
	return ModelRoutingValidationResult{
		Contract:    *raw.Contract,
		OK:          *raw.OK,
		Diagnostics: diagnostics,
	}, nil
}
