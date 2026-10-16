//go:build darwin

package shellinstaller

// Darwin arm64 mirrors of the Linux pins: the same release versions, other
// platform assets. Each archive pin equals its publisher checksum (nodejs.org
// SHASUMS256.txt, gentle-ai checksums.txt, ripgrep .sha256, the GitHub release
// asset digest for fd); member pins are derived from those exact archives.

const privateColdPath = "/dist/v24.18.0/node-v24.18.0-darwin-arm64.tar.gz"
const privateColdURL = "https://nodejs.org" + privateColdPath
const privateColdSize int64 = 52087559
const privateColdSHA = "e1a97e14c99c803e96c7339403282ea05a499c32f8d83defe9ef5ec66f979ed1"

// Node Mach-O pin derived from the archive-pinned v24.18.0 darwin-arm64 member.
const privateNativeNodeSize int64 = 120965360
const privateNativeNodeSHA = "ee6fb0e015284d83a91e8ec5213f43a157f8a392b58555301682892ba928c04a"

const userBinarySize int64 = 16047186
const userBinarySHA = "18a9f7fae55d85c95684b6d512a4a148d0cb24a856325f72573c34caf65159eb"
const userNativeManifest = `{"version":"4.0.0","asset":"gentle-ai_4.0.0_darwin_arm64.tar.gz","assetSha256":"d2159caf6d68f367b18830ece6af71ef26963d5f5320d7df6a794773f45cc7e9","binarySha256":"18a9f7fae55d85c95684b6d512a4a148d0cb24a856325f72573c34caf65159eb"}` + "\n"

var userToolSources = []userToolSource{
	{"fd", "sharkdp/fd", "v10.5.0", "fd-v10.5.0-aarch64-apple-darwin", "b67e1836c468e42e411984b56e52fa7abec08c2bd22c867398e7cc134aac5e12", 1334374},
	{"rg", "BurntSushi/ripgrep", "15.2.0", "ripgrep-15.2.0-aarch64-apple-darwin", "3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4", 1764284},
}
