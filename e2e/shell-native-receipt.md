# Native producer receipt

The Linux amd64 native producer can transport its original successful result and
installation locator in `native-installation.json`. This is data, not permission
to publish: consumers must first establish physical producer settlement and then
use the existing physical payload authentication.

The writer exclusively creates a durable 0600 file in the private producer root.
The reader requires anchored no-follow access, current user/group ownership,
unchanged file identity and metadata, and at most 64 KiB. It preserves the original
raw result so terminal action validation is not lost during transport.

Focused checks: `TestUserNativeProducerReceipt*` and
`TestIndependentNativeReceiptFIFO`. The latter verifies nonblocking FIFO refusal
without modifying the original entry. Private namespace orchestration is covered
separately by the native coordinator qualification.