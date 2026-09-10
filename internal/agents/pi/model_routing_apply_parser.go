package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type ModelRoutingApplyOutcome string

const (
	ModelRoutingApplyOutcomeSuccess            ModelRoutingApplyOutcome = "success"
	ModelRoutingApplyOutcomeValidationFailure  ModelRoutingApplyOutcome = "validation-failure"
	ModelRoutingApplyOutcomeUnavailableRuntime ModelRoutingApplyOutcome = "unavailable-runtime"
	ModelRoutingApplyOutcomePersistenceFailure ModelRoutingApplyOutcome = "persistence-failure"
	ModelRoutingApplyOutcomePartial            ModelRoutingApplyOutcome = "partial"
)

type ModelRoutingApplyFailure struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}
type ModelRoutingApplyMaterialization struct {
	Affected  []string                   `json:"affected"`
	Succeeded []string                   `json:"succeeded"`
	Failed    []ModelRoutingApplyFailure `json:"failed"`
}
type ModelRoutingApplyResult struct {
	Contract        string                            `json:"contract"`
	OK              bool                              `json:"ok"`
	Outcome         ModelRoutingApplyOutcome          `json:"outcome"`
	Target          *ModelRoutingTarget               `json:"target,omitempty"`
	ConfigPath      *string                           `json:"configPath,omitempty"`
	Saved           bool                              `json:"saved"`
	Diagnostics     []ModelRoutingDiagnostic          `json:"diagnostics"`
	Materialization *ModelRoutingApplyMaterialization `json:"materialization,omitempty"`
}
type ModelRoutingApplyResponse struct {
	Version   int                     `json:"version"`
	Contract  string                  `json:"contract"`
	Operation ModelRoutingOperation   `json:"operation"`
	OK        bool                    `json:"ok"`
	ExitClass string                  `json:"exitClass"`
	Result    ModelRoutingApplyResult `json:"result"`
}
type modelRoutingApplyResultJSON struct {
	Contract        *string                   `json:"contract"`
	OK              *bool                     `json:"ok"`
	Outcome         *ModelRoutingApplyOutcome `json:"outcome"`
	Target          json.RawMessage           `json:"target"`
	ConfigPath      json.RawMessage           `json:"configPath"`
	Saved           *bool                     `json:"saved"`
	Diagnostics     json.RawMessage           `json:"diagnostics"`
	Materialization json.RawMessage           `json:"materialization"`
}
type modelRoutingApplyMaterializationJSON struct {
	Affected  json.RawMessage `json:"affected"`
	Succeeded json.RawMessage `json:"succeeded"`
	Failed    json.RawMessage `json:"failed"`
}

func ParseModelRoutingApplyResponse(payload []byte) (ModelRoutingApplyResponse, error) {
	envelope, err := decodeModelRoutingOperationEnvelope(payload, ModelRoutingOperationApply)
	if err != nil {
		return ModelRoutingApplyResponse{}, err
	}
	result, err := decodeModelRoutingApplyResult(envelope.Result)
	if err != nil {
		return ModelRoutingApplyResponse{}, modelRoutingResponseError(ModelRoutingResponseErrorMalformed, envelope.Operation, ModelRoutingOperationApply, ErrModelRoutingResponseMalformed, err)
	}
	return ModelRoutingApplyResponse{Version: envelope.Version, Contract: envelope.Contract, Operation: envelope.Operation, OK: envelope.OK, ExitClass: envelope.ExitClass, Result: result}, nil
}
func decodeModelRoutingApplyResult(data []byte) (ModelRoutingApplyResult, error) {
	var raw modelRoutingApplyResultJSON
	if err := decodeModelRoutingApplyObject(data, &raw, "result"); err != nil {
		return ModelRoutingApplyResult{}, err
	}
	if raw.Contract == nil {
		return ModelRoutingApplyResult{}, errors.New("model-routing apply result contract is required")
	}
	if *raw.Contract != modelRoutingContract {
		return ModelRoutingApplyResult{}, fmt.Errorf("invalid model-routing apply result contract %q", *raw.Contract)
	}
	if raw.OK == nil {
		return ModelRoutingApplyResult{}, errors.New("model-routing apply result ok is required")
	}
	if raw.Outcome == nil {
		return ModelRoutingApplyResult{}, errors.New("model-routing apply result outcome is required")
	}
	if err := modelRoutingLiteral("apply outcome", string(*raw.Outcome), string(ModelRoutingApplyOutcomeSuccess), string(ModelRoutingApplyOutcomeValidationFailure), string(ModelRoutingApplyOutcomeUnavailableRuntime), string(ModelRoutingApplyOutcomePersistenceFailure), string(ModelRoutingApplyOutcomePartial)); err != nil {
		return ModelRoutingApplyResult{}, err
	}
	if raw.Saved == nil {
		return ModelRoutingApplyResult{}, errors.New("model-routing apply result saved is required")
	}
	if err := requireModelRoutingApplyValue(raw.Diagnostics, "diagnostics"); err != nil {
		return ModelRoutingApplyResult{}, err
	}
	diagnostics, err := decodeModelRoutingApplyDiagnostics(raw.Diagnostics)
	if err != nil {
		return ModelRoutingApplyResult{}, err
	}
	result := ModelRoutingApplyResult{Contract: *raw.Contract, OK: *raw.OK, Outcome: *raw.Outcome, Saved: *raw.Saved, Diagnostics: diagnostics}
	if len(raw.Target) != 0 {
		if err := requireModelRoutingApplyValue(raw.Target, "target"); err != nil {
			return ModelRoutingApplyResult{}, err
		}
		var target ModelRoutingTarget
		if err := json.Unmarshal(raw.Target, &target); err != nil {
			return ModelRoutingApplyResult{}, err
		}
		if err := modelRoutingLiteral("apply target", string(target), string(ModelRoutingTargetGlobal), string(ModelRoutingTargetProject)); err != nil {
			return ModelRoutingApplyResult{}, err
		}
		result.Target = &target
	}
	if len(raw.ConfigPath) != 0 {
		if err := requireModelRoutingApplyValue(raw.ConfigPath, "configPath"); err != nil {
			return ModelRoutingApplyResult{}, err
		}
		var configPath string
		if err := json.Unmarshal(raw.ConfigPath, &configPath); err != nil {
			return ModelRoutingApplyResult{}, err
		}
		result.ConfigPath = &configPath
	}
	if len(raw.Materialization) != 0 {
		if err := requireModelRoutingApplyValue(raw.Materialization, "materialization"); err != nil {
			return ModelRoutingApplyResult{}, err
		}
		materialization, err := decodeModelRoutingApplyMaterialization(raw.Materialization)
		if err != nil {
			return ModelRoutingApplyResult{}, err
		}
		result.Materialization = &materialization
	}
	return result, nil
}
func decodeModelRoutingApplyObject(data []byte, value any, name string) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return fmt.Errorf("model-routing apply %s must be one object", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
func requireModelRoutingApplyValue(value []byte, name string) error {
	if len(bytes.TrimSpace(value)) == 0 || modelRoutingNull(value) {
		return fmt.Errorf("model-routing apply result %s is required and non-null", name)
	}
	return nil
}
func decodeModelRoutingApplyDiagnostics(data []byte) ([]ModelRoutingDiagnostic, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var diagnostics []ModelRoutingDiagnostic
	if err := decoder.Decode(&diagnostics); err != nil {
		return nil, err
	}
	if err := validateModelRoutingDiagnostics(data, diagnostics); err != nil {
		return nil, err
	}
	return diagnostics, nil
}
func decodeModelRoutingApplyMaterialization(data []byte) (ModelRoutingApplyMaterialization, error) {
	var raw modelRoutingApplyMaterializationJSON
	if err := decodeModelRoutingApplyObject(data, &raw, "materialization"); err != nil {
		return ModelRoutingApplyMaterialization{}, err
	}
	for name, data := range map[string]json.RawMessage{"affected": raw.Affected, "succeeded": raw.Succeeded, "failed": raw.Failed} {
		if err := requireModelRoutingApplyValue(data, name); err != nil {
			return ModelRoutingApplyMaterialization{}, err
		}
	}
	affected, err := decodeModelRoutingApplyStrings(raw.Affected)
	if err != nil {
		return ModelRoutingApplyMaterialization{}, fmt.Errorf("affected: %w", err)
	}
	succeeded, err := decodeModelRoutingApplyStrings(raw.Succeeded)
	if err != nil {
		return ModelRoutingApplyMaterialization{}, fmt.Errorf("succeeded: %w", err)
	}
	failed, err := decodeModelRoutingApplyFailures(raw.Failed)
	if err != nil {
		return ModelRoutingApplyMaterialization{}, fmt.Errorf("failed: %w", err)
	}
	return ModelRoutingApplyMaterialization{Affected: affected, Succeeded: succeeded, Failed: failed}, nil
}
func decodeModelRoutingApplyStrings(data []byte) ([]string, error) {
	if err := modelRoutingStrings(data); err != nil {
		return nil, err
	}
	var values []string
	_ = json.Unmarshal(data, &values)
	return values, nil
}
func decodeModelRoutingApplyFailures(data []byte) ([]ModelRoutingApplyFailure, error) {
	values, err := modelRoutingArray(data)
	if err != nil {
		return nil, err
	}
	failures := make([]ModelRoutingApplyFailure, 0, len(values))
	for _, value := range values {
		if _, err := modelRoutingObjectFields(value, []string{"target", "message"}, nil); err != nil {
			return nil, err
		}
		var failure ModelRoutingApplyFailure
		if err := decodeModelRoutingApplyObject(value, &failure, "materialization failure"); err != nil {
			return nil, err
		}
		failures = append(failures, failure)
	}
	return failures, nil
}
