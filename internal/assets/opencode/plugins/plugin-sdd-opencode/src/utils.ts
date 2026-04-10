/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Utilidades generales del plugin
 */

import { ActiveProfileState } from "./types";

export function formatContext(tokens: number | null): string {
  if (!tokens || typeof tokens !== "number") return "ctx: N/A";
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1).replace(/\.0$/, "")}M ctx`;
  if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}k ctx`;
  return `${tokens} ctx`;
}

export function formatMemoryDate(value: string | undefined): string {
  if (!value) return "Sin fecha";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function truncateText(value: string, max = 120): string {
  if (!value) return "";
  return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}

export function isManagedSddAgent(agentName: string): boolean {
  return agentName.startsWith("sdd-");
}

export function resolveModelInfo(api: any, modelId: string): string {
  if (!modelId) return "Sin asignar";
  const [providerId, ...rest] = modelId.split("/");
  const modelKey = rest.join("/");
  const provider = api.state.provider.find((p: any) => p.id === providerId);
  const model = provider?.models?.[modelKey];
  const ctx = model?.limit?.context;
  const ctxStr = ctx ? ` (${formatContext(ctx)})` : "";
  return `${modelId}${ctxStr}`;
}

export function parseActiveProfileFromRaw(raw: string, api: any): ActiveProfileState | null {
  try {
    const config = JSON.parse(raw);
    const providers = api.state.provider || [];
    const agentConfigs = config.agent || config.model || {};
    const agentNames = Object.keys(agentConfigs);

    if (agentNames.length === 0) return null;

    const firstAgent =
      agentNames.find((name) => isManagedSddAgent(name) && agentConfigs[name]?.model) ||
      agentNames.find((name) => agentConfigs[name]?.model) ||
      agentNames[0];

    const modelId = agentConfigs[firstAgent]?.model;
    if (!modelId) return null;

    const [providerId, ...rest] = modelId.split("/");
    const modelKey = rest.join("/");
    const provider = providers.find((p: any) => p.id === providerId);

    if (!provider) {
      return { modelId, modelName: modelId, providerName: providerId, contextLimit: null };
    }

    const modelDef = provider.models?.[modelKey];
    return {
      modelId,
      modelName: modelDef?.name || modelKey,
      providerName: provider.name || provider.id,
      contextLimit: modelDef?.limit?.context || null,
    };
  } catch {
    return null;
  }
}
