//go:build linux

package shellinstaller

// Node ELF pin independently derived from the fully archive-pinned v24.18.0
// Linux-x64 regular member, not from a mutable installed bootstrap inventory.
const privateNativeNodeSize int64 = 123655872
const privateNativeNodeSHA = "41a74efb34cbde5c7632cdac0cf8bd1a14d0b8d73dc1e82755014d9a9ce70f5c"
const privateNativeResolverSHA = "cbdf5deac8b7a85ab1253dbd049953aeb192a7d1f7987f9206916ab449c10a92"
const privateNativeInvoke = `import {installGentleAi} from './scripts/gentle-ai-installer.mjs'; const packageRoot=process.cwd(); await installGentleAi({packageRoot});`
