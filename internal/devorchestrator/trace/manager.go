package trace

import (
	"fmt"
)

// TraceabilityError represents a hard error when traceability validation fails.
type TraceabilityError struct {
	Message string
}

func (e *TraceabilityError) Error() string {
	return fmt.Sprintf("traceability breach: %s", e.Message)
}

// Manager defines the interface for the traceability validation component.
type Manager interface {
	ValidatePhaseTransition(sourceArtifactPath, destArtifactPath string) error
}

type defaultManager struct{}

// NewManager creates a new traceability manager instance.
func NewManager() Manager {
	return &defaultManager{}
}

// ValidatePhaseTransition validates that the destination artifact implements or originates from the source artifact's ID.
func (m *defaultManager) ValidatePhaseTransition(sourceArtifactPath, destArtifactPath string) error {
	sourceNode, err := ParseTraceability(sourceArtifactPath)
	if err != nil {
		return fmt.Errorf("failed to parse source artifact: %w", err)
	}

	if sourceNode == nil || sourceNode.ID == "" {
		// If the source has no ID, we can't trace it. We could either fail or ignore.
		return &TraceabilityError{Message: fmt.Sprintf("source artifact %s lacks an ID", sourceArtifactPath)}
	}

	destNode, err := ParseTraceability(destArtifactPath)
	if err != nil {
		return fmt.Errorf("failed to parse destination artifact: %w", err)
	}

	if destNode == nil {
		return &TraceabilityError{Message: fmt.Sprintf("destination artifact %s lacks traceability metadata", destArtifactPath)}
	}

	// Check if destNode references sourceNode.ID
	found := false
	for _, id := range destNode.Implements {
		if id == sourceNode.ID {
			found = true
			break
		}
	}
	
	if !found {
		for _, id := range destNode.OriginatesFrom {
			if id == sourceNode.ID {
				found = true
				break
			}
		}
	}

	if !found {
		return &TraceabilityError{
			Message: fmt.Sprintf("artifact %s does not declare implementation of source ID %s", destArtifactPath, sourceNode.ID),
		}
	}

	return nil
}
