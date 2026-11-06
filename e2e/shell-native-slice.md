# Operation-owned Linux native slice

`newUserNativeSlice` validates the existing canonical user-manager runtime/socket and trusted busctl/systemctl executables, then requests one random alphanumeric transient slice with exact aggregate3GiB/swap0/CPU1/Tasks64 limits. No persistent units, implicit parent slices, configuration edits or retries are created.

Create the slice before launching either coordinator or sealed child. `serviceArg` returns their common Slice argument only after physical cgroup2 identity/limits readback; `member` checks actual current descendant membership and existing leaf kernel controls. A name, argument or caller-supplied receipt alone is not authorization.

Retain the returned handle even when creation reports uncertainty: the selected name may already exist. Do not retry creation or blindly stop an unbound slice. The caller retains evidence and settles/cancels both owned leaf services first. `settle` refuses populated/replaced/unreadable state, stops only its bound empty slice and verifies removal. It does not kill workers or discover/recover ambient operations.

The ordinary entry is not wired here. Private498881 qualification established the native mechanism; unit tests cover construction/refusal and the private manager integration test is explicitly opt-in. Actual producer execution, TUI cancellation and installed acceptance remain separate proof.
