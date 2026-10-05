package permissions

import "encoding/json"

// openCodeOverlayJSON mirrors the Gentle Pi safety model for OpenCode (and
// Kilocode): everything is allowed by design; recognized destructive commands
// are confirmed (Pi confirm) or denied outright (Pi hard deny); remote shell
// utilities are denied outright across their escape forms (#4324); and
// secret-bearing paths cannot be read or modified ("edit" covers the edit,
// write, and patch tools).
//
// OpenCode resolves these wildcard rules in order and the LAST matching rule
// wins. `*` spans any characters, including `/`, and `?` matches exactly one.
// The settings writer keeps "*" first and writes the remaining rules of a fresh
// document in byte order, so precedence follows byte order: every deny must
// sort after the ask it overrides. "$" and `"` sort before "*", which is why
// $HOME targets are spelled "?HOME" and a quoted "$HOME" is "??HOME?".
// TestOpenCodePermissionsMirrorPiSafetyModel resolves rules from the written
// file and pins this.
//
// A wildcard rule cannot tokenize a shell command the way Pi does, so wrappers
// other than sudo (env, nohup, timeout, xargs, VAR=value), nested shells,
// absolute executable paths, unusual flag permutations, and `rm -rf /*` (which
// cannot be denied without denying every absolute path) are config limits.
// The remote shell utilities are the carve-out (#4324): their canonical
// install prefixes, root-level path (which also reaches backslash escapes,
// because the matcher normalizes backslashes to forward slashes), command
// and exec resolution wrappers, and direct env execution (env tool ...) are
// enumerated explicitly, mirroring the Claude Code deny list. A human who
// wants a utility back re-enables it in their own config.
var openCodeOverlayJSON = buildOpenCodeOverlayJSON()

var (
	// openCodeRecursiveRMFlags are the recursive rm flag spellings guarded,
	// including the common verbose permutations.
	openCodeRecursiveRMFlags = []string{
		"-r", "-R", "-rf", "-fr", "-Rf", "-fR",
		"-rfv", "-frv", "-rvf", "-vrf", "-Rfv", "--recursive",
	}
	// openCodeSudoRMFlags are the recursive rm spellings also guarded behind
	// sudo; the full cross product would bloat the user's settings file.
	openCodeSudoRMFlags = []string{"-r", "-R", "-rf", "-fr", "-Rf", "-fR"}
	// openCodeProtectedRMTargets are the Pi root targets: filesystem root,
	// home, and the current or parent directory.
	openCodeProtectedRMTargets = []string{
		"/", "~", "~/*", "?HOME", "?HOME/*", "?{HOME}", "??HOME?",
		".", "./", "..", "../",
	}
	// openCodeTrailingRMTargets are also denied as the last of several rm
	// operands (for example `rm -rf build /`).
	openCodeTrailingRMTargets = []string{"/", "~", "?HOME"}
	// openCodeGitPrefixes cover git invoked directly or with -C <dir>.
	openCodeGitPrefixes = []string{"git ", "git -C * "}
	// openCodeRemoteCommands keep their approval (#4324).
	openCodeRemoteCommands = []string{"ssh", "scp", "sftp", "rsync"}

	// openCodeRemotePathPrefixes lists the canonical install prefixes of the
	// OpenSSH client and rsync across the supported platforms (FHS Linux /bin
	// and /usr/bin, custom /usr/local/bin, Apple Silicon /opt/homebrew/bin,
	// NixOS /run/current-system/sw/bin).
	openCodeRemotePathPrefixes = []string{
		"/bin/",
		"/usr/bin/",
		"/usr/local/bin/",
		"/opt/homebrew/bin/",
		"/run/current-system/sw/bin/",
	}
	// openCodeDatabaseClients and openCodeDestructiveSQL confirm destructive
	// SQL passed to a database client, as Pi does.
	openCodeDatabaseClients = []string{"psql", "mysql", "mariadb", "sqlite3"}
	openCodeDestructiveSQL  = []string{
		"DROP TABLE", "DROP DATABASE", "DROP SCHEMA", "TRUNCATE",
		"drop table", "drop database", "drop schema", "truncate",
	}
	// openCodeSensitivePaths are the secret-bearing paths Gentle Pi guards for
	// its read, write, and edit tools: .env files and their variants, .ssh,
	// .credentials, secrets and keychain directories, AWS and GitHub CLI
	// credential files, and private key/certificate bundles.
	openCodeSensitivePaths = []string{
		"*.env", "*.env.*", "**/.env", "**/.env.*",
		".env_*", "*/.env_*", ".env-*", "*/.env-*", ".env/*", "*/.env/*",
		"secrets", "secrets/*", "**/secrets", "**/secrets/**",
		".ssh", ".ssh/*", "**/.ssh", "**/.ssh/**",
		".credentials", ".credentials/*", "**/.credentials", "**/.credentials/**",
		"**/Library/Keychains/**",
		".aws/credentials", "**/.aws/credentials",
		".config/gh/hosts.yml", "**/.config/gh/hosts.yml",
		".config/gh/hosts.yaml", "**/.config/gh/hosts.yaml",
		"**/credentials.json",
		"*.pem", "**/*.pem", "*.key", "**/*.key", "*.p12", "*.pfx",
	}
)

func buildOpenCodeOverlayJSON() []byte {
	bash := map[string]string{"*": "allow"}
	ask := func(pattern string) { bash[pattern] = "ask" }
	deny := func(pattern string) { bash[pattern] = "deny" }

	recursiveRM := func(prefix string, flags []string) {
		for _, flag := range flags {
			ask(prefix + "rm " + flag + " *")
			for _, target := range openCodeProtectedRMTargets {
				deny(prefix + "rm " + flag + " " + target)
			}
			for _, target := range openCodeTrailingRMTargets {
				deny(prefix + "rm " + flag + " * " + target)
			}
		}
	}
	recursiveRM("", openCodeRecursiveRMFlags)
	recursiveRM("sudo ", openCodeSudoRMFlags)
	for _, prefix := range []string{"", "sudo "} {
		ask(prefix + "find *-delete*")
		deny(prefix + "chmod -R 777*")
		deny(prefix + "chown -R *")
	}
	for _, git := range openCodeGitPrefixes {
		ask(git + "push")
		ask(git + "push *")
		ask(git + "rebase *")
		ask(git + "branch -D *")
		deny(git + "reset --hard")
		deny(git + "reset --hard *")
		deny(git + "reset * --hard*")
		// Forced clean: an "f" inside the first flag cluster (up to three
		// characters, which also covers --force) or a later "-f" / "-?f"
		// cluster. A broader "-*f*" would deny dry runs whose paths contain
		// an "f", such as `git clean -n -- fixtures/`.
		deny(git + "clean -f*")
		deny(git + "clean -?f*")
		deny(git + "clean -??f*")
		deny(git + "clean * -f*")
		deny(git + "clean * -?f*")
		deny(git + "push --force*")
		deny(git + "push * --force*")
		deny(git + "push -f*")
		deny(git + "push * -f*")
		// Short-flag clusters are enumerated: "-?f*" would also match
		// --follow-tags, since "?" cannot exclude "-".
		for _, cluster := range []string{"-uf", "-fu"} {
			deny(git + "push " + cluster + "*")
			deny(git + "push * " + cluster + "*")
		}
	}
	ask("npm publish")
	ask("npm publish *")
	// #4324 deny policy: the remote shell utilities are denied by default in
	// their bare and wildcard forms plus the escape forms a bare prefix rule
	// cannot reach, instead of being asked per command.
	for _, command := range openCodeRemoteCommands {
		deny(command)
		deny(command + " *")
		for _, prefix := range openCodeRemotePathPrefixes {
			deny(prefix + command + " *")
		}
		deny("/" + command + " *")
		deny("command " + command + " *")
		deny("exec " + command + " *")
		deny("env " + command + " *")
	}
	for _, client := range openCodeDatabaseClients {
		for _, statement := range openCodeDestructiveSQL {
			ask(client + " *" + statement + "*")
		}
	}

	paths := map[string]string{"*": "allow"}
	for _, path := range openCodeSensitivePaths {
		paths[path] = "deny"
	}

	overlay, err := json.MarshalIndent(map[string]any{
		"permission": map[string]any{"bash": bash, "read": paths, "edit": paths},
	}, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(overlay, '\n')
}
