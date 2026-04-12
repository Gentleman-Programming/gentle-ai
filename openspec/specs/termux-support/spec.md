# Termux Support Specification

## Purpose
Define the requirements for `gentle-ai` to operate correctly within the Termux environment on Android, ensuring environment detection, path resolution, and execution safety.

## Requirements

### Requirement: Platform Detection (Termux)
The system MUST identify when it is running inside Termux to apply the correct environment overrides.

#### Scenario: Detect Termux environment
- GIVEN the target OS input `GOOS` is `android`
- WHEN the system runs `detectFromInputs`
- THEN the `PlatformProfile.LinuxDistro` SHALL be `termux`
- AND `PlatformProfile.Supported` SHALL be `true`

### Requirement: Prefix-Aware Path Resolution
The system SHALL dynamically resolve system paths by prepending the Termux `$PREFIX` when running in the Termux distro.

#### Scenario: Resolve shell path in Termux
- GIVEN the system distro is `termux`
- AND the environment variable `PREFIX` is `/data/data/com.termux/files/usr`
- WHEN the system resolves the path for `bash`
- THEN the result SHALL be `/data/data/com.termux/files/usr/bin/bash`

#### Scenario: Fallback to standard path on non-Termux Linux
- GIVEN the system distro is `ubuntu`
- WHEN the system resolves the path for `bash`
- THEN the result SHALL be `/usr/bin/bash` (no prefix added)

### Requirement: PATH Persistence (Termux Shells)
The system MUST persist the installation directory to the user's PATH by modifying Termux-specific shell configuration files.

#### Scenario: Add to PATH in .bashrc
- GIVEN the system distro is `termux`
- AND the shell is `bash`
- WHEN `AddToUserPath` is called with `/data/data/com.termux/files/home/.gentle-ai/bin`
- THEN the system SHALL append the export command to `~/.bashrc`
- AND it SHALL NOT attempt to call `powershell` or modify the Windows registry.
