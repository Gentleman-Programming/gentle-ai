package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// MaxModelRoutingResponseBytes is the shared maximum for model-routing JSON responses.
const MaxModelRoutingResponseBytes = 256 << 10

// decodeModelRoutingInspectionJSON decodes syntax and canonical DTO shape only.
// Later response handling owns contract and semantic validation.
func decodeModelRoutingInspectionJSON(data []byte) (ModelRoutingInspection, map[string]json.RawMessage, error) {
	if len(data) > MaxModelRoutingResponseBytes {
		return ModelRoutingInspection{}, nil, errors.New("model-routing inspection is oversized")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return ModelRoutingInspection{}, nil, errors.New("model-routing inspection must be one object")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, otherObject); err != nil {
		return ModelRoutingInspection{}, nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			err = errors.New("model-routing inspection has trailing JSON")
		}
		return ModelRoutingInspection{}, nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ModelRoutingInspection{}, nil, err
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var result ModelRoutingInspection
	if err := decoder.Decode(&result); err != nil {
		return ModelRoutingInspection{}, nil, err
	}
	return result, raw, nil
}
