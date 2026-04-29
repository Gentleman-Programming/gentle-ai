package pi

type Contract string

const (
	ContractBinaryInstallMethod Contract = "binary-install-method"
	ContractConfigRoot          Contract = "config-root"
	ContractSettingsSchema      Contract = "settings-schema"
	ContractMCPShape            Contract = "mcp-shape"
	ContractCommandPromptLayout Contract = "command-prompt-layout"
	ContractModelCacheAuth      Contract = "model-cache-auth"
	ContractProfileSupport      Contract = "profile-support"
)

var requiredContracts = []Contract{
	ContractBinaryInstallMethod,
	ContractConfigRoot,
	ContractSettingsSchema,
	ContractMCPShape,
	ContractCommandPromptLayout,
	ContractModelCacheAuth,
	ContractProfileSupport,
}

type ContractStatus string

const (
	ContractStatusPass    ContractStatus = "pass"
	ContractStatusFail    ContractStatus = "fail"
	ContractStatusUnknown ContractStatus = "unknown"
)

type ContractCheck struct {
	Contract Contract
	Status   ContractStatus
	Reason   string
}

type RolloutReadiness struct {
	Ready    bool
	Blockers []ContractCheck
}

func EvaluateRolloutReadiness(matrix map[Contract]ContractStatus) RolloutReadiness {
	result := RolloutReadiness{Ready: true}

	for _, contract := range requiredContracts {
		status, ok := matrix[contract]
		if !ok {
			result.Ready = false
			result.Blockers = append(result.Blockers, ContractCheck{
				Contract: contract,
				Status:   ContractStatusUnknown,
				Reason:   "missing evidence for required contract",
			})
			continue
		}

		switch status {
		case ContractStatusPass:
			continue
		case ContractStatusFail:
			result.Ready = false
			result.Blockers = append(result.Blockers, ContractCheck{
				Contract: contract,
				Status:   ContractStatusFail,
				Reason:   "status fail blocks rollout",
			})
		default:
			result.Ready = false
			result.Blockers = append(result.Blockers, ContractCheck{
				Contract: contract,
				Status:   ContractStatusUnknown,
				Reason:   "status unknown blocks rollout",
			})
		}
	}

	return result
}
