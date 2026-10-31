package shellinstaller

// userPNPMManifest is optional: absent metadata keeps the existing npm contract.
// Paths are root-relative; source inode stamps are deliberately not persisted.
type userPNPMManifest struct {
	PackageRoot  string `json:"package_root"`
	ShellVersion string `json:"shell_version"`
	PeerVersion  string `json:"peer_version"`
	CodeSeal     string `json:"code_seal"`
}
