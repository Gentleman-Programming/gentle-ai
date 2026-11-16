package shellinstaller

// UserReplacedBackup is one discovered candidate of a replaced installation
// beside the active target. Listing enumerates exact canonical siblings
// without validating manifests, sums each tree's bytes within a bound, and
// marks the lexically latest candidate; removal keeps the validated retention
// primitives, so a listed tree is never deleted by an unvalidated name.
type UserReplacedBackup struct {
	Path     string
	Stamp    string
	Bytes    int64
	Complete bool
	Latest   bool
}
