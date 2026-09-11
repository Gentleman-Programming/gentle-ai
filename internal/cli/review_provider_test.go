package cli

import (
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

// TestReviewProviderAdapterFor pins #3258: it covers reviewer provider adapter selection
// and host-mediated or unregistered-adapter refusals across runtime identities.
func TestReviewProviderAdapterFor(t *testing.T) {
	contract, err := reviewerprovider.ContractFor(reviewerprovider.RoleLens)
	if err != nil {
		t.Fatalf("ContractFor(RoleLens) = %v", err)
	}

	tests := []struct {
		name        string
		agent       model.AgentID
		wantAdapter func(t *testing.T, adapter reviewerprovider.Adapter)
		wantErr     string
	}{
		{
			name:  "claude returns compiled adapter",
			agent: model.AgentClaudeCode,
			wantAdapter: func(t *testing.T, adapter reviewerprovider.Adapter) {
				t.Helper()
				if _, ok := adapter.(*reviewerprovider.ClaudeAdapter); !ok {
					t.Fatalf("adapter type = %T, want *reviewerprovider.ClaudeAdapter", adapter)
				}
			},
		},
		{
			name:  "codex returns compiled adapter",
			agent: model.AgentCodex,
			wantAdapter: func(t *testing.T, adapter reviewerprovider.Adapter) {
				t.Helper()
				if _, ok := adapter.(*reviewerprovider.CodexAdapter); !ok {
					t.Fatalf("adapter type = %T, want *reviewerprovider.CodexAdapter", adapter)
				}
			},
		},
		{
			name:    "opencode returns host-mediated refusal",
			agent:   model.AgentOpenCode,
			wantErr: `reviewer provider runtime "opencode" is host-mediated; launch the provider-issued OpenCode reviewer task`,
		},
		{
			name:    "pi returns distinct host-mediated refusal",
			agent:   model.AgentPi,
			wantErr: `reviewer provider runtime "pi" is host-mediated; launch the provider-issued Pi reviewer task`,
		},
		{
			name:    "unsupported runtime returns unregistered adapter refusal",
			agent:   model.AgentID("unsupported"),
			wantErr: `reviewer provider runtime "unsupported" has no registered adapter`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := reviewProviderAdapterFor(contract, tt.agent)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("reviewProviderAdapterFor() error = nil, want error %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("reviewProviderAdapterFor() error = %q, want %q", err.Error(), tt.wantErr)
				}
				if adapter != nil {
					t.Fatalf("reviewProviderAdapterFor() adapter = %#v, want nil", adapter)
				}
				return
			}
			if err != nil {
				t.Fatalf("reviewProviderAdapterFor() unexpected error = %v", err)
			}
			if tt.wantAdapter != nil {
				tt.wantAdapter(t, adapter)
			}
		})
	}

	t.Run("rejects contract without required transport capability before runtime selection", func(t *testing.T) {
		contractWithoutTransport := reviewerprovider.Contract{
			Role: reviewerprovider.RoleLens,
		}
		for _, agent := range []model.AgentID{
			model.AgentClaudeCode,
			model.AgentCodex,
			model.AgentOpenCode,
			model.AgentPi,
			model.AgentID("unsupported"),
		} {
			adapter, err := reviewProviderAdapterFor(contractWithoutTransport, agent)
			if err == nil {
				t.Fatalf("reviewProviderAdapterFor(%s) error = nil, want transport capability refusal", agent)
			}
			want := `reviewer provider role "lens" does not permit the compiled transport`
			if err.Error() != want {
				t.Fatalf("reviewProviderAdapterFor(%s) error = %q, want %q", agent, err.Error(), want)
			}
			if adapter != nil {
				t.Fatalf("reviewProviderAdapterFor(%s) adapter = %#v, want nil", agent, adapter)
			}
		}
	})
}
