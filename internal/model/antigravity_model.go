package model

// AntigravityModelTier is the agy custom-agent `model:` tier for a role.
type AntigravityModelTier string

const (
	AntigravityModelInherit   AntigravityModelTier = "inherit"
	AntigravityModelFlashLite AntigravityModelTier = "flash_lite"
	AntigravityModelFlash     AntigravityModelTier = "flash"
	AntigravityModelPro       AntigravityModelTier = "pro"
)

// AntigravityModelRoles are the roles with a native agy agent whose `model:`
// frontmatter Gentle AI renders. Antigravity has no RDD review agents.
var AntigravityModelRoles = []string{"jd-judge-a", "jd-judge-b", "jd-fix-agent"}

// Valid reports whether the tier is one agy accepts in agent frontmatter.
func (t AntigravityModelTier) Valid() bool {
	switch t {
	case AntigravityModelInherit, AntigravityModelFlashLite, AntigravityModelFlash, AntigravityModelPro:
		return true
	default:
		return false
	}
}

// AntigravityModelID maps a tier to the agy frontmatter value. Unknown or
// missing tiers inherit the session model, matching agy's own default.
func AntigravityModelID(tier AntigravityModelTier) string {
	if !tier.Valid() {
		return string(AntigravityModelInherit)
	}
	return string(tier)
}

// AntigravityModelPresetInherit keeps every native agent on the session model.
func AntigravityModelPresetInherit() map[string]AntigravityModelTier {
	return antigravityModelPreset(AntigravityModelInherit, AntigravityModelInherit)
}

// AntigravityModelPresetPerformance runs judges and the fix agent on pro.
func AntigravityModelPresetPerformance() map[string]AntigravityModelTier {
	return antigravityModelPreset(AntigravityModelPro, AntigravityModelPro)
}

// AntigravityModelPresetEconomy runs judges on flash and fixes on flash_lite.
func AntigravityModelPresetEconomy() map[string]AntigravityModelTier {
	return antigravityModelPreset(AntigravityModelFlash, AntigravityModelFlashLite)
}

func antigravityModelPreset(judges, fix AntigravityModelTier) map[string]AntigravityModelTier {
	return map[string]AntigravityModelTier{"jd-judge-a": judges, "jd-judge-b": judges, "jd-fix-agent": fix}
}

// AntigravityModelTiersToStrings converts tiers for state.json.
func AntigravityModelTiersToStrings(m map[string]AntigravityModelTier) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for role, tier := range m {
		out[role] = string(tier)
	}
	return out
}

// AntigravityModelTiersFromStrings restores persisted tiers, dropping unknown
// values so their roles render as inherit.
func AntigravityModelTiersFromStrings(m map[string]string) map[string]AntigravityModelTier {
	out := make(map[string]AntigravityModelTier, len(m))
	for role, value := range m {
		if tier := AntigravityModelTier(value); tier.Valid() {
			out[role] = tier
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
