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

const userBinarySize int64 = 17462210
const userBinarySHA = "8ea490287ef796848b0d305cf2b26265e360d3f400efc016972844498fb87097"
const userNativeManifest = `{"version":"4.2.0","asset":"gentle-ai_4.2.0_darwin_arm64.tar.gz","assetSha256":"d5e9a6521de5f08a393635f083f64dc7afc1b8e97884f7aab5ae5eb1f17c7f37","binarySha256":"8ea490287ef796848b0d305cf2b26265e360d3f400efc016972844498fb87097"}` + "\n"

var userToolSources = []userToolSource{
	{"fd", "sharkdp/fd", "v10.5.0", "fd-v10.5.0-aarch64-apple-darwin", "b67e1836c468e42e411984b56e52fa7abec08c2bd22c867398e7cc134aac5e12", 1334374},
	{"rg", "BurntSushi/ripgrep", "15.2.0", "ripgrep-15.2.0-aarch64-apple-darwin", "3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4", 1764284},
}
