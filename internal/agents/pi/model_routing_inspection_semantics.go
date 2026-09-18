package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

func decodeModelRoutingInspection(data []byte) (ModelRoutingInspection, error) {
	result, document, err := decodeModelRoutingInspectionJSON(data)
	if err != nil {
		return ModelRoutingInspection{}, err
	}
	if err := validateModelRoutingInspection(document, result); err != nil {
		return ModelRoutingInspection{}, err
	}
	return result, nil
}

func modelRoutingNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
func modelRoutingArray(raw json.RawMessage) ([]json.RawMessage, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	if values == nil {
		return nil, errors.New("model-routing value must be an array")
	}
	return values, nil
}

func modelRoutingObjectFields(raw json.RawMessage, required, optional []string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("model-routing value must be an object")
	}
	if err := modelRoutingFields(object, required, optional); err != nil {
		return nil, err
	}
	return object, nil
}
func modelRoutingFields(object map[string]json.RawMessage, required, optional []string) error {
	for _, field := range required {
		raw, ok := object[field]
		if !ok || modelRoutingNull(raw) {
			return fmt.Errorf("model-routing required field %q is absent or null", field)
		}
	}
	for _, field := range optional {
		if raw, ok := object[field]; ok && modelRoutingNull(raw) {
			return fmt.Errorf("model-routing optional field %q is null", field)
		}
	}
	return nil
}

func modelRoutingStrings(raw json.RawMessage) error {
	values, err := modelRoutingArray(raw)
	if err != nil {
		return err
	}
	for _, value := range values {
		if modelRoutingNull(value) {
			return errors.New("model-routing string array contains null")
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return errors.New("model-routing string array contains a non-string")
		}
	}
	return nil
}
func modelRoutingEach(raw json.RawMessage, required, optional []string, fn func(int, map[string]json.RawMessage) error) error {
	values, err := modelRoutingArray(raw)
	if err != nil {
		return err
	}
	for i, value := range values {
		object, err := modelRoutingObjectFields(value, required, optional)
		if err != nil {
			return err
		}
		if err := fn(i, object); err != nil {
			return err
		}
	}
	return nil
}

func modelRoutingAll(checks ...error) error {
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}
func modelRoutingLiteral(field, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("invalid model-routing %s %q", field, value)
}
func modelRoutingTrue(field string, raw json.RawMessage) error {
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("true")) {
		return fmt.Errorf("model-routing %s must be true", field)
	}
	return nil
}

func validateModelRoutingInspection(document map[string]json.RawMessage, result ModelRoutingInspection) error {
	if err := modelRoutingFields(document, []string{"contract", "targets", "assignments", "agents", "providers", "models", "diagnostics"}, []string{"context"}); err != nil {
		return err
	}
	if result.Contract != modelRoutingContract {
		return fmt.Errorf("invalid model-routing contract %q", result.Contract)
	}
	if raw, ok := document["context"]; ok {
		if err := validateModelRoutingContext(raw, *result.Context); err != nil {
			return err
		}
	}
	return modelRoutingAll(
		validateModelRoutingTargets(document["targets"], result.Targets),
		validateModelRoutingAssignments(document["assignments"], result.Assignments),
		validateModelRoutingAgents(document["agents"], result.Agents), modelRoutingStrings(document["providers"]),
		validateModelRoutingModels(document["models"], result.Models), validateModelRoutingDiagnostics(document["diagnostics"], result.Diagnostics),
	)
}

func validateModelRoutingContext(raw json.RawMessage, context ModelRoutingContext) error {
	object, err := modelRoutingObjectFields(raw, []string{"cwd", "agentDir", "target", "global", "project"}, nil)
	if err != nil {
		return err
	}
	return modelRoutingAll(
		modelRoutingLiteral("context target", string(context.Target), string(ModelRoutingTargetGlobal), string(ModelRoutingTargetProject)),
		validateModelRoutingResolvedTarget(object["global"], context.Global), validateModelRoutingResolvedTarget(object["project"], context.Project),
	)
}
func validateModelRoutingResolvedTarget(raw json.RawMessage, target ModelRoutingResolvedTarget) error {
	if _, err := modelRoutingObjectFields(raw, []string{"target", "configPath", "profilePath"}, nil); err != nil {
		return err
	}
	return modelRoutingLiteral("resolved target", string(target.Target), string(ModelRoutingTargetGlobal), string(ModelRoutingTargetProject))
}
func validateModelRoutingTargets(raw json.RawMessage, targets map[ModelRoutingTarget]ModelRoutingTargetInspection) error {
	object, err := modelRoutingObjectFields(raw, nil, nil)
	if err != nil {
		return err
	}
	for key, rawTarget := range object {
		target, err := modelRoutingObjectFields(rawTarget, []string{"provenance", "assignments"}, nil)
		if err != nil {
			return err
		}
		value := targets[ModelRoutingTarget(key)]
		if err := modelRoutingAll(
			modelRoutingLiteral("target key", key, string(ModelRoutingTargetGlobal), string(ModelRoutingTargetProject)),
			validateModelRoutingProvenance(target["provenance"], value.Provenance), validateModelRoutingAssignments(target["assignments"], value.Assignments),
		); err != nil {
			return err
		}
	}
	return nil
}

func validateModelRoutingProvenance(raw json.RawMessage, provenance ModelRoutingProvenance) error {
	if _, err := modelRoutingObjectFields(raw, []string{"target", "source", "status", "configPath", "profilePath"}, nil); err != nil {
		return err
	}
	return modelRoutingAll(
		modelRoutingLiteral("provenance target", string(provenance.Target), string(ModelRoutingTargetGlobal), string(ModelRoutingTargetProject)),
		modelRoutingLiteral("provenance source", string(provenance.Source), string(ModelRoutingProvenanceSourceGlobal), string(ModelRoutingProvenanceSourceProject), string(ModelRoutingProvenanceSourceMissing), string(ModelRoutingProvenanceSourceInvalid)),
		modelRoutingLiteral("provenance status", string(provenance.Status), string(ModelRoutingProvenanceStatusValid), string(ModelRoutingProvenanceStatusMissing), string(ModelRoutingProvenanceStatusInvalid)),
	)
}

func validateModelRoutingAssignments(raw json.RawMessage, assignments map[string]ModelRoutingAssignment) error {
	object, err := modelRoutingObjectFields(raw, nil, nil)
	if err != nil {
		return err
	}
	for name, value := range object {
		if err := validateModelRoutingAssignment(value, assignments[name]); err != nil {
			return err
		}
	}
	return nil
}
func validateModelRoutingAssignment(raw json.RawMessage, assignment ModelRoutingAssignment) error {
	if _, err := modelRoutingObjectFields(raw, []string{"inheritModel", "inheritThinking"}, []string{"model", "thinking"}); err != nil {
		return err
	}
	if assignment.Thinking == nil {
		return nil
	}
	return modelRoutingLiteral("assignment thinking", string(*assignment.Thinking), string(ModelRoutingThinkingOff), string(ModelRoutingThinkingMinimal), string(ModelRoutingThinkingLow), string(ModelRoutingThinkingMedium), string(ModelRoutingThinkingHigh), string(ModelRoutingThinkingXHigh), string(ModelRoutingThinkingMax))
}

func validateModelRoutingAgents(raw json.RawMessage, agents []ModelRoutingAgent) error {
	return modelRoutingEach(raw, []string{"name", "source", "configurable"}, []string{"filePath", "assignment"}, func(i int, object map[string]json.RawMessage) error {
		if err := modelRoutingAll(
			modelRoutingLiteral("agent source", string(agents[i].Source), string(ModelRoutingAgentSourceProject), string(ModelRoutingAgentSourceUser), string(ModelRoutingAgentSourceBuiltin)),
			modelRoutingTrue("agent configurable", object["configurable"]),
		); err != nil {
			return err
		}
		if rawAssignment, ok := object["assignment"]; ok {
			if agents[i].Assignment == nil {
				return errors.New("model-routing agent assignment is invalid")
			}
			return validateModelRoutingAssignment(rawAssignment, *agents[i].Assignment)
		}
		return nil
	})
}

func validateModelRoutingModels(raw json.RawMessage, models []ModelRoutingModel) error {
	return modelRoutingEach(raw, []string{"canonicalId", "provider", "modelId", "name", "catalog", "configured", "authConfigured", "available", "authenticated", "operational", "availability", "reasoning", "capabilities"}, []string{"api", "supportedThinkingLevels"}, func(i int, object map[string]json.RawMessage) error {
		model := models[i]
		var levelsErr error
		if levels, ok := object["supportedThinkingLevels"]; ok {
			levelsErr = modelRoutingStrings(levels)
		}
		return modelRoutingAll(
			modelRoutingTrue("model catalog", object["catalog"]),
			modelRoutingLiteral("operational", string(model.Operational), string(ModelRoutingOperationalAuthenticated), string(ModelRoutingOperationalUnavailable), string(ModelRoutingOperationalUnknown)),
			modelRoutingLiteral("availability", string(model.Availability), string(ModelRoutingAvailabilityCatalog), string(ModelRoutingAvailabilityConfigured), string(ModelRoutingAvailabilityAuthenticated), string(ModelRoutingAvailabilityUnknown)),
			validateModelRoutingCapabilities(object["capabilities"]), levelsErr,
		)
	})
}

func validateModelRoutingCapabilities(raw json.RawMessage) error {
	object, err := modelRoutingObjectFields(raw, []string{"reasoning", "input"}, []string{"contextWindow", "maxTokens", "thinkingLevels"})
	if err != nil {
		return err
	}
	var levelsErr error
	if levels, ok := object["thinkingLevels"]; ok {
		levelsErr = modelRoutingStrings(levels)
	}
	return modelRoutingAll(modelRoutingStrings(object["input"]), levelsErr)
}
func validateModelRoutingDiagnostics(raw json.RawMessage, diagnostics []ModelRoutingDiagnostic) error {
	return modelRoutingEach(raw, []string{"code", "message", "severity"}, []string{"path"}, func(i int, _ map[string]json.RawMessage) error {
		return modelRoutingLiteral("diagnostic severity", string(diagnostics[i].Severity), string(ModelRoutingDiagnosticSeverityError), string(ModelRoutingDiagnosticSeverityWarning), string(ModelRoutingDiagnosticSeverityInfo))
	})
}
