//go:build !linux

package privatefile

// WriteNewReport fails closed on platforms without the verified private writer.
func WriteNewReport(absoluteDestination string, payload []byte) error {
	return ErrUnsupported
}
