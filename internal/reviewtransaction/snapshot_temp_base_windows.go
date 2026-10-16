package reviewtransaction

func snapshotTempBaseSafe(_ string) bool {
	// Keep the established Git-local behavior until process-temp ancestor
	// qualification has an equivalent Windows ACL proof.
	// guard:population snapshot-temp-windows-ancestry fail-closed: Without equivalent ancestor ACL proof, preserve Git-local allocation rather than admitting unqualified scratch; repository inputs remain supported.
	return false
}
