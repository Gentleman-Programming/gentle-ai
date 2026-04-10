/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Configuración y paths del plugin
 */

import * as fs from "node:fs";
import * as path from "node:path";
import * as os from "node:os";
import { execFileSync } from "node:child_process";

export type Paths = {
  configRoot: string;
  profilesDir: string;
  configPath: string;
  backupPath: string;
};

export function resolvePaths(): Paths {
  const home = os.homedir();
  const xdgConfig = process.env.XDG_CONFIG_HOME || path.join(home, ".config");
  const configRoot = path.join(xdgConfig, "opencode");

  return {
    configRoot,
    profilesDir: path.join(configRoot, "profiles"),
    configPath: path.join(configRoot, "opencode.json"),
    backupPath: path.join(configRoot, "opencode.json.bak"),
  };
}

export function ensureProfilesDir(): void {
  const { profilesDir } = resolvePaths();
  if (!fs.existsSync(profilesDir)) {
    try {
      fs.mkdirSync(profilesDir, { recursive: true });
    } catch (e) {}
  }
}

export function resolveProjectName(api: any): string {
  return resolveProjectCandidates(api)[0] || "unknown";
}

export function resolveProjectCandidates(api: any): string[] {
  const directory = api?.state?.path?.directory || process.cwd();
  const candidates: string[] = [];

  try {
    const remote = execFileSync("git", ["-C", directory, "remote", "get-url", "origin"], {
      encoding: "utf-8",
      timeout: 2000,
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();

    if (remote) {
      const repoName = remote.replace(/\.git$/, "").split(/[/:]/).pop()?.trim().toLowerCase();
      if (repoName) candidates.push(repoName);
    }
  } catch {}

  try {
    const root = execFileSync("git", ["-C", directory, "rev-parse", "--show-toplevel"], {
      encoding: "utf-8",
      timeout: 2000,
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();

    if (root) {
      const rootName = path.basename(root)?.trim().toLowerCase();
      if (rootName) candidates.push(rootName);
    }
  } catch {}

  const dirName = path.basename(directory)?.trim().toLowerCase();
  if (dirName) candidates.push(dirName);

  return [...new Set(candidates.filter(Boolean))];
}

export function resolveWorkspaceRoot(api: any): string {
  return api?.state?.path?.directory || process.cwd();
}
