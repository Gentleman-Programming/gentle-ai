package model

// StrategyInstructionsFile writes a VS Code .instructions.md file with YAML frontmatter.
const StrategyInstructionsFile SystemPromptStrategy = StrategyAppendToFile + 1

// StrategySteeringFile writes a Kiro steering file with inclusion: always frontmatter.
const StrategySteeringFile SystemPromptStrategy = StrategyInstructionsFile + 1
