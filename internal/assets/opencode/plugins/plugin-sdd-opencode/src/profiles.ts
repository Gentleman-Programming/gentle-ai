/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Lógica de perfiles SDD
 */

import * as fs from "node:fs";
import * as path from "node:path";
import { ProfileModels } from "./types";
import { isManagedSddAgent } from "./utils";
import { resolvePaths, ensureProfilesDir } from "./config";

export function isSddProfile(fileName: string): boolean {
  return fileName.endsWith(".json");
}

export function extractSddAgentModels(config: any): ProfileModels {
  const agents = config?.agent || {};
  return Object.fromEntries(
    Object.entries(agents).filter(
      ([name, value]: any) => isManagedSddAgent(name) && typeof value?.model === "string" && value.model
    ).map(([name, value]: any) => [name, value.model])
  );
}

export function readProfileModels(profilePath: string): ProfileModels {
  const raw = JSON.parse(fs.readFileSync(profilePath, "utf-8"));

  if (raw && typeof raw === "object" && !Array.isArray(raw) && !raw.agent) {
    return Object.fromEntries(
      Object.entries(raw)
        .filter(
          ([name, value]: any) =>
            isManagedSddAgent(name) &&
            ((typeof value === "string" && value) || (typeof value?.model === "string" && value.model))
        )
        .map(([name, value]: any) => [name, typeof value === "string" ? value : value.model])
    );
  }

  return extractSddAgentModels(raw);
}

export function writeProfileModels(profilePath: string, models: ProfileModels): void {
  fs.writeFileSync(profilePath, JSON.stringify(models, null, 2));
}

export function detectActiveProfileFile(files: string[], api: any): string | undefined {
  const activeAgents = (api.state.config as any)?.agent || {};
  const { profilesDir } = resolvePaths();
  const activeSddAgents = Object.fromEntries(
    Object.entries(activeAgents)
      .filter(([name, value]: any) => isManagedSddAgent(name) && typeof value?.model === "string" && value.model)
      .map(([name, value]: any) => [name, value.model])
  );

  for (const file of files) {
    try {
      const profileModels = readProfileModels(path.join(profilesDir, file));
      const keys = Object.keys(profileModels);
      if (keys.length === 0) continue;

      if (keys.length !== Object.keys(activeSddAgents).length) continue;

      const allMatch = keys.every((agentName) => {
        const profileModel = profileModels[agentName];
        const activeModel = activeSddAgents[agentName];
        return profileModel && profileModel === activeModel;
      });

      if (allMatch) return file;
    } catch (e) {}
  }
  return undefined;
}

function applyProfileModelsToConfig(currentConfig: any, profileModels: ProfileModels): any {
  const nextConfig = JSON.parse(JSON.stringify(currentConfig || {}));
  if (!nextConfig.agent) nextConfig.agent = {};

  for (const [agentName, modelId] of Object.entries(profileModels)) {
    nextConfig.agent[agentName] = {
      ...(nextConfig.agent[agentName] || {}),
      model: modelId,
    };
  }

  return nextConfig;
}

export async function activateProfileFile(api: any, profilePath: string, profileName: string): Promise<any | null> {
  const { configPath } = resolvePaths();
  try {
    const profileModels = readProfileModels(profilePath);

    if (Object.keys(profileModels).length === 0) {
      api.ui.toast({
        title: "Activation Failed",
        message: "El perfil no tiene modelos SDD para aplicar",
        variant: "error",
      });
      return;
    }

    // IMPORTANT:
    // Use on-disk config as source-of-truth to preserve declarative links like
    // {file:...}. Runtime `global.config.get()` may return resolved content,
    // and sending that back can materialize/inline file contents.
    let currentConfig: any;
    if (fs.existsSync(configPath)) {
      currentConfig = JSON.parse(fs.readFileSync(configPath, "utf-8"));
    } else {
      const globalConfigResult = await api.client.global.config.get();
      currentConfig = globalConfigResult?.data || {};
    }
    const nextConfig = applyProfileModelsToConfig(currentConfig, profileModels);

    const result = await api.client.global.config.update({
      config: nextConfig,
    });

    if (result?.error) throw new Error(result.error.message || "No se pudo actualizar la config global runtime");

    // IMPORTANT:
    // Do NOT rewrite opencode.json from plugin-side after profile switch.
    // The runtime config API is the source of truth for persistence/format.
    // Rewriting here causes full-file churn (indent/style drift) and can
    // materialize resolved/default fields that were not explicitly set.
    return result?.data || nextConfig;
  } catch (err: any) {
    api.ui.toast({ title: "Activation Failed", message: err.message, variant: "error" });
    return null;
  }
}

export function listProfileFiles(): string[] {
  const { profilesDir } = resolvePaths();
  ensureProfilesDir();
  try {
    return fs.readdirSync(profilesDir).filter((f) => isSddProfile(f));
  } catch {
    return [];
  }
}

export function deleteProfileFile(fileName: string): void {
  const { profilesDir } = resolvePaths();
  const profilePath = path.join(profilesDir, fileName);
  fs.unlinkSync(profilePath);
}

export function renameProfileFile(oldFileName: string, newFileName: string): void {
  const { profilesDir } = resolvePaths();
  const oldPath = path.join(profilesDir, oldFileName);
  const newPath = path.join(profilesDir, newFileName);
  fs.renameSync(oldPath, newPath);
}
