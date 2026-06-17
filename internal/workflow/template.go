package workflow

// BuiltInTemplates exposes the workflow templates that ship with gentle-ai.
// Other tools (CLI, docs, TUI) discover templates by walking this map.
//
// Adding a template here is a non-breaking change: existing workflows keep
// working. The CLI's `workflow init` picks a template by name and writes it
// out via WriteFile.
//
// Note: the built-in SDD workflow is NOT in this map because it is a system
// workflow, not a user-initializable template. Use SDDTemplate() to access it.
var BuiltInTemplates = map[string]WorkflowDefinition{
	"paper-review":              paperReviewTemplate(),
	"academic-article-review": academicArticleReviewTemplate(),
}

// TemplateNames returns a stable, sorted list of built-in template names.
// Useful for `gentle-ai workflow list` (not part of PR 1) and for tests.
func TemplateNames() []string {
	names := make([]string, 0, len(BuiltInTemplates))
	for name := range BuiltInTemplates {
		names = append(names, name)
	}
	// stable order
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}

// Template returns a deep copy of the named built-in template, or false if
// no such template exists. Always copy so callers can mutate the result
// without affecting the registry.
func Template(name string) (WorkflowDefinition, bool) {
	src, ok := BuiltInTemplates[name]
	if !ok {
		return WorkflowDefinition{}, false
	}
	return cloneDefinition(src), true
}

// paperReviewTemplate implements the eight-phase scientific paper review DAG
// described in the proposal. The phase set, dependencies, and skill choices
// must stay byte-for-byte compatible with the example in proposal.md; if you
// change one, change both.
func paperReviewTemplate() WorkflowDefinition {
	phases := []WorkflowPhase{
		{
			ID:             "identify-gap",
			Name:           "Identify Research Gap",
			Artifact:       "gap-analysis.md",
			DependsOn:      []string{},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "gap-clarity",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "Gap is clear, justified, and scoped",
				},
			},
		},
		{
			ID:             "evaluate-writing",
			Name:           "Evaluate Writing Quality",
			Artifact:       "writing-review.md",
			DependsOn:      []string{"identify-gap"},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "scientific-english",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "Formal register, precise terminology, consistent tense",
				},
			},
		},
		{
			ID:             "verify-data",
			Name:           "Verify Data Matches Text",
			Artifact:       "data-review.md",
			DependsOn:      []string{"identify-gap"},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "data-consistency",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "Quantitative claims match tables/figures",
				},
			},
		},
		{
			ID:             "review-figures",
			Name:           "Review Figures",
			Artifact:       "figure-review.md",
			DependsOn:      []string{"identify-gap"},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "figure-correctness",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "Axes labeled, error bars present, captions self-contained",
				},
			},
		},
		{
			ID:             "check-style",
			Name:           "Check Journal Style",
			Artifact:       "style-review.md",
			DependsOn:      []string{"evaluate-writing"},
			RequiredSkills: []string{"academic-researcher", "latex-formatting"},
			ValidationGates: []ValidationGate{
				{
					ID:       "journal-style",
					Type:     GateAgentEvaluated,
					Skill:    "latex-formatting",
					Criteria: "Matches journal template, citation format, word limits",
				},
			},
		},
		{
			ID:             "verify-code",
			Name:           "Verify Code Reflects Paper",
			Artifact:       "code-review.md",
			DependsOn:      []string{"verify-data"},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "code-reflects-paper",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "Scripts produce reported numbers, dependencies versioned",
				},
			},
		},
		{
			ID:             "structural-review",
			Name:           "Structural Review",
			Artifact:       "structure-review.md",
			DependsOn:      []string{"evaluate-writing", "check-style"},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "structure-correct",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "Logical flow, clear section purpose, smooth transitions",
				},
			},
		},
		{
			ID:       "final-report",
			Name:     "Compile Final Report",
			Artifact: "final-report.md",
			DependsOn: []string{
				"identify-gap",
				"evaluate-writing",
				"verify-data",
				"review-figures",
				"check-style",
				"verify-code",
				"structural-review",
			},
			RequiredSkills: []string{"academic-researcher"},
			ValidationGates: []ValidationGate{
				{
					ID:       "final-completeness",
					Type:     GateAgentEvaluated,
					Skill:    "academic-researcher",
					Criteria: "References all artifacts, gives recommendation with justification",
				},
			},
		},
	}

	strictTDD := false
	return WorkflowDefinition{
		Name:         "paper-review",
		Version:      SupportedSchemaVersion,
		ProducesCode: false,
		Validation: WorkflowValidation{
			StrictTDD: &strictTDD,
		},
		Phases: phases,
	}
}

// academicArticleReviewTemplate implements the 11-phase academic article review
// DAG described in the spec (8 conceptual phases with parallel sub-phases 3A/3B
// and 5A/5B/5C). Phase names are in Spanish per the user's requirement.
func academicArticleReviewTemplate() WorkflowDefinition {
	academicResearcher := "academic-researcher"
	phases := []WorkflowPhase{
		{
			ID:             "global-reading",
			Name:           "Lectura global y comprensión",
			Artifact:       "global-reading.md",
			DependsOn:      []string{},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "global-reading-complete",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Complete understanding of the article's scope, claims, and structure",
				},
			},
		},
		{
			ID:             "initial-diagnosis",
			Name:           "Diagnóstico inicial (fortalezas y debilidades)",
			Artifact:       "initial-diagnosis.md",
			DependsOn:      []string{"global-reading"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "diagnosis-balanced",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Identifies both strengths and weaknesses with evidence",
				},
			},
		},
		{
			ID:             "scientific-review",
			Name:           "Revisión científica (contenido)",
			Artifact:       "scientific-review.md",
			DependsOn:      []string{"initial-diagnosis"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "scientific-rigor",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Methodology, results, and conclusions are scientifically sound",
				},
			},
		},
		{
			ID:             "narrative-review",
			Name:           "Revisión narrativa (escritura)",
			Artifact:       "narrative-review.md",
			DependsOn:      []string{"initial-diagnosis"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "narrative-clarity",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Writing is clear, well-structured, and follows academic conventions",
				},
			},
		},
		{
			ID:             "improvement-plan",
			Name:           "Plan de mejoras priorizado",
			Artifact:       "improvement-plan.md",
			DependsOn:      []string{"scientific-review", "narrative-review"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "plan-prioritized",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Improvements are prioritized by impact and feasibility",
				},
			},
		},
		{
			ID:             "scientific-improvements",
			Name:           "Mejoras científicas",
			Artifact:       "scientific-improvements.md",
			DependsOn:      []string{"improvement-plan"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "scientific-improvements-applied",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Scientific improvements address the identified weaknesses",
				},
			},
		},
		{
			ID:             "experiment-improvements",
			Name:           "Mejoras de experimentos",
			Artifact:       "experiment-improvements.md",
			DependsOn:      []string{"improvement-plan"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "experiment-improvements-applied",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Experimental design improvements are concrete and justified",
				},
			},
		},
		{
			ID:             "writing-improvements",
			Name:           "Mejoras de redacción",
			Artifact:       "writing-improvements.md",
			DependsOn:      []string{"improvement-plan"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "writing-improvements-applied",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Writing improvements enhance clarity, flow, and academic tone",
				},
			},
		},
		{
			ID:             "coherence-review",
			Name:           "Revisión integral de coherencia global",
			Artifact:       "coherence-review.md",
			DependsOn:      []string{"scientific-improvements", "experiment-improvements", "writing-improvements"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "global-coherence",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "All improvements integrate coherently; no contradictions or gaps",
				},
			},
		},
		{
			ID:             "reviewer-simulation",
			Name:           "Simulación de reviewers (Reviewer 1/2/3)",
			Artifact:       "reviewer-simulation.md",
			DependsOn:      []string{"coherence-review"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "reviewer-perspectives",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "Three distinct reviewer personas provide realistic, constructive feedback",
				},
			},
		},
		{
			ID:             "submission-preparation",
			Name:           "Preparación de envío (checklist de conferencia)",
			Artifact:       "submission-preparation.md",
			DependsOn:      []string{"reviewer-simulation"},
			RequiredSkills: []string{academicResearcher},
			ValidationGates: []ValidationGate{
				{
					ID:       "submission-ready",
					Type:     GateAgentEvaluated,
					Skill:    academicResearcher,
					Criteria: "All conference checklist items are verified and complete",
				},
			},
		},
	}

	strictTDD := false
	return WorkflowDefinition{
		Name:         "academic-article-review",
		Version:      SupportedSchemaVersion,
		ProducesCode: false,
		Validation: WorkflowValidation{
			StrictTDD: &strictTDD,
		},
		Phases: phases,
	}
}

// SDDTemplate returns the built-in SDD workflow definition. SDD is a system
// workflow, not a user-initializable template — it is NOT registered in
// BuiltInTemplates. Callers that need the SDD workflow definition (e.g.
// internal/components/sdd/inject.go for strict_tdd resolution) use this
// function to access it.
func SDDTemplate() WorkflowDefinition {
	phases := []WorkflowPhase{
		{
			ID:        "sdd-explore",
			Name:      "Explore",
			Artifact:  "exploration.md",
			DependsOn: []string{},
		},
		{
			ID:        "sdd-propose",
			Name:      "Propose",
			Artifact:  "proposal.md",
			DependsOn: []string{"sdd-explore"},
		},
		{
			ID:        "sdd-spec",
			Name:      "Spec",
			Artifact:  "spec.md",
			DependsOn: []string{"sdd-propose"},
		},
		{
			ID:        "sdd-design",
			Name:      "Design",
			Artifact:  "design.md",
			DependsOn: []string{"sdd-spec"},
		},
		{
			ID:        "sdd-tasks",
			Name:      "Tasks",
			Artifact:  "tasks.md",
			DependsOn: []string{"sdd-design"},
		},
		{
			ID:        "sdd-apply",
			Name:      "Apply",
			Artifact:  "apply-progress.md",
			DependsOn: []string{"sdd-tasks"},
		},
		{
			ID:        "sdd-verify",
			Name:      "Verify",
			Artifact:  "verify-report.md",
			DependsOn: []string{"sdd-apply"},
		},
		{
			ID:        "sdd-archive",
			Name:      "Archive",
			Artifact:  "archive.md",
			DependsOn: []string{"sdd-verify"},
		},
	}

	strictTDD := true
	return WorkflowDefinition{
		Name:         "sdd",
		Version:      SupportedSchemaVersion,
		ProducesCode: true,
		Validation: WorkflowValidation{
			StrictTDD: &strictTDD,
		},
		Phases: phases,
	}
}

// cloneDefinition is a deep-enough copy of a WorkflowDefinition for the
// registry use case. Slices are duplicated so callers can mutate them.
func cloneDefinition(def WorkflowDefinition) WorkflowDefinition {
	out := def
	if def.Phases != nil {
		out.Phases = make([]WorkflowPhase, len(def.Phases))
		for i, p := range def.Phases {
			out.Phases[i] = clonePhase(p)
		}
	}
	return out
}

func clonePhase(p WorkflowPhase) WorkflowPhase {
	out := p
	if p.DependsOn != nil {
		out.DependsOn = append([]string(nil), p.DependsOn...)
	}
	if p.RequiredSkills != nil {
		out.RequiredSkills = append([]string(nil), p.RequiredSkills...)
	}
	if p.ValidationGates != nil {
		out.ValidationGates = make([]ValidationGate, len(p.ValidationGates))
		for i, g := range p.ValidationGates {
			out.ValidationGates[i] = g
		}
	}
	return out
}
