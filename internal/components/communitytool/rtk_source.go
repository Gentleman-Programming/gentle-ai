package communitytool

import "github.com/gentleman-programming/gentle-ai/v3/internal/model"

const (
	rtkReleaseTag           = "v0.49.0"
	rtkReleaseCommit        = "b1c0dc00649c50fbe8930f849c800d4d6ca12091"
	rtkTelemetryDisabled    = "RTK_TELEMETRY_DISABLED=1"
	rtkDocumentedKillSwitch = "RTK_DISABLED=1"
	rtkChecksumsSHA256      = "a5ff3570fe196a21e09a249c1777665d6ed887630d1c7c66de1154d4340d3ad0"
)

type rtkUserMutationPathClass string

const rtkUserMutationPathInstruction rtkUserMutationPathClass = "instruction-file"

type rtkUserMutationPath struct {
	Class    rtkUserMutationPathClass
	Template string
}

// rtkSetupContracts records source-derived init metadata. It neither resolves a
// home directory nor constructs or executes a command.
type rtkSetupContract struct {
	Agent            model.AgentID
	Invocation       string
	UserMutationPath rtkUserMutationPath
}

var rtkSetupContracts = []rtkSetupContract{
	{model.AgentClaudeCode, "init -g --auto-patch", rtkUserMutationPath{rtkUserMutationPathInstruction, "~/.claude/CLAUDE.md"}},
	{model.AgentOpenCode, "init -g --opencode", rtkUserMutationPath{rtkUserMutationPathInstruction, "~/.config/opencode/plugins/rtk.ts"}},
	{model.AgentCodex, "init -g --codex", rtkUserMutationPath{rtkUserMutationPathInstruction, "~/.codex/AGENTS.md"}},
	{model.AgentPi, "init -g --agent " + "pi --auto-patch", rtkUserMutationPath{rtkUserMutationPathInstruction, "PI_CODING_AGENT_DIR/extensions/rtk.ts"}},
}

type rtkPlatform string

const (
	rtkPlatformDarwinARM64 rtkPlatform = "darwin/arm64"
	rtkPlatformLinuxARM64  rtkPlatform = "linux/arm64"
	rtkPlatformDarwinAMD64 rtkPlatform = "darwin/amd64"
	rtkPlatformLinuxAMD64  rtkPlatform = "linux/amd64"
	rtkPlatformWindows     rtkPlatform = "windows"
)

type rtkCandidateAsset struct {
	Platform            rtkPlatform
	Name                string
	URL                 string
	ExecutableMember    string
	SizeBytes           int64
	SHA256              string
	ChecksumSHA256      string
	ExecutableSizeBytes int64
	ExecutableSHA256    string
}

func rtkReleaseAsset(platform rtkPlatform, name string, size int64, digest string, executableSize int64, executableDigest string) rtkCandidateAsset {
	return rtkCandidateAsset{
		Platform: platform, Name: name,
		URL:              "https://github.com/rtk-ai/rtk/releases/download/" + rtkReleaseTag + "/" + name,
		ExecutableMember: "rtk", SizeBytes: size, SHA256: digest, ChecksumSHA256: rtkChecksumsSHA256,
		ExecutableSizeBytes: executableSize, ExecutableSHA256: executableDigest,
	}
}

// rtkCandidateAssets preserves all pinned v0.49.0 release facts. Windows stays
// a candidate only; rtkEnabledPlatforms is the production admission boundary.
var rtkCandidateAssets = []rtkCandidateAsset{
	rtkReleaseAsset(rtkPlatformDarwinARM64, "rtk-aarch64-apple-darwin.tar.gz", 4_062_392, "bbbfebabb22686993a80da731aa4d5d35116fb8ae24abb00608efa028e13ae01", 8_342_080, "055ef1cd1aa0afb96c854bddf43d24af10dcac5a6288ed44519cb1cc10b093a9"),
	rtkReleaseAsset(rtkPlatformLinuxARM64, "rtk-aarch64-unknown-linux-gnu.tar.gz", 4_402_148, "c8ea4b6560841e73157c134fd4a3293914c6ede42e786ee985cf491fde691ba7", 9_202_032, "4fa443857061b1226a21a8503112adba078a5c7ca3f7fd5919782afacf5566ca"),
	rtkReleaseAsset(rtkPlatformDarwinAMD64, "rtk-x86_64-apple-darwin.tar.gz", 4_445_957, "d297388f4a8a786e79abe5f55b80451725bfe8c5835b4736c05d7cff4d68f627", 9_721_224, "1a28052aa71b4d865c3346de6e20216a791ad7e011845ced078b2f70745ce983"),
	rtkReleaseAsset(rtkPlatformLinuxAMD64, "rtk-x86_64-unknown-linux-musl.tar.gz", 4_791_180, "7278231dfd7e6a730a4ab7f847b195bcf02289c2d57622b0dab75a6411100c8f", 10_888_832, "a051b22361c7cfa36022bc3f06bb41cdc88e58a07263dc340d8bd3468c41befe"),
	{
		Platform: rtkPlatformWindows, Name: "rtk-x86_64-pc-windows-msvc.zip",
		URL:              "https://github.com/rtk-ai/rtk/releases/download/v0.49.0/rtk-x86_64-pc-windows-msvc.zip",
		ExecutableMember: "rtk.exe", SizeBytes: 4_448_627,
		SHA256: "cb971046598f0e8bd51f6c27780fcdd2c39a4c459a811bd95b0d77ba8c0d7c9f", ChecksumSHA256: rtkChecksumsSHA256,
	},
}

var rtkEnabledPlatforms = []rtkPlatform{rtkPlatformDarwinARM64, rtkPlatformLinuxARM64, rtkPlatformDarwinAMD64, rtkPlatformLinuxAMD64}
