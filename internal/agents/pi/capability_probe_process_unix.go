//go:build !windows

package pi

import "context"

func runModelRoutingProcess(context.Context, string, []byte, ModelRoutingProcessOptions) (ModelRoutingProcessResult, error) {
	return ModelRoutingProcessResult{}, transportError(TransportErrorUnsupportedPlatform, ErrTransportUnsupportedPlatform)
}
