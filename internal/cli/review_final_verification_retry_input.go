package cli

import (
	"context"
	"errors"
	"io"
	"time"
)

const finalVerificationIncidentInputLimit = 16 << 10

var errFinalVerificationIncidentInputTooLarge = errors.New("final-verification incident exceeds the native size limit")

func readFinalVerificationIncidentInput(ctx context.Context, path string) ([]byte, error) {
	file, restore, err := openFinalVerificationIncidentInput(ctx, path)
	if err != nil {
		return nil, err
	}
	interrupted := make(chan struct{})
	stopInterrupt := context.AfterFunc(ctx, func() {
		cancelFacadeArtifactManifestInput(file)
		_ = file.SetReadDeadline(time.Now())
		_ = file.Close()
		close(interrupted)
	})
	payload, readErr := io.ReadAll(io.LimitReader(file, finalVerificationIncidentInputLimit+1))
	if stopInterrupt() {
		_ = file.Close()
	} else {
		<-interrupted
	}
	restoreErr := restore()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if readErr != nil {
		return nil, readErr
	}
	if restoreErr != nil {
		return nil, restoreErr
	}
	if len(payload) > finalVerificationIncidentInputLimit {
		return nil, errFinalVerificationIncidentInputTooLarge
	}
	return payload, nil
}
