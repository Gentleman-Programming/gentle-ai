/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Dialogs de UI del plugin
 */

import * as fs from "node:fs";
import * as path from "node:path";
import { NAV_CATEGORY } from "./types";
import {
  resolveModelInfo,
  formatMemoryDate,
  truncateText,
  parseActiveProfileFromRaw,
  formatContext,
} from "./utils";
import { resolvePaths, ensureProfilesDir } from "./config";
import {
  listProfileFiles,
  readProfileModels,
  writeProfileModels,
  extractSddAgentModels,
  detectActiveProfileFile,
  activateProfileFile,
  deleteProfileFile,
  renameProfileFile,
} from "./profiles";
import { deleteProjectMemory, listProjectMemories } from "./memories";
import { setActiveProfile } from "./state";

function showMemoryDetail(api: any, memory: any) {
  const sanitizeMemoryDisplayText = (value: string): string =>
    value
      .replace(/\*\*(.*?)\*\*/g, "$1")
      .replace(/`([^`]+)`/g, "$1")
      .replace(/[“”]/g, '"')
      .replace(/[‘’]/g, "'")
      .replace(/→/g, "->");

  const wrapDisplayText = (value: string, max = 52): string[] => {
    if (!value) return [" "];
    const words = sanitizeMemoryDisplayText(value).split(/\s+/).filter(Boolean);
    if (words.length === 0) return [" "];

    const lines: string[] = [];
    let current = "";

    for (const word of words) {
      if (!current) {
        current = word;
        continue;
      }

      if (`${current} ${word}`.length <= max) {
        current = `${current} ${word}`;
        continue;
      }

      lines.push(current);
      current = word;
    }

    if (current) lines.push(current);
    return lines.length > 0 ? lines : [value];
  };

  const title = memory.title || memory.topic_key || `Memory #${memory.id}`;
  const metadata = `[${(memory.type || "manual").toUpperCase()}] ${formatMemoryDate(
    memory.updated_at || memory.created_at
  )} · ${memory.scope || "project"}`;
  const contentLines = (memory.content || "Sin contenido")
    .split("\n")
    .flatMap((line: string) => wrapDisplayText(line || " "));

  api.ui.dialog.replace(() => (
    <api.ui.DialogSelect
      title={truncateText(title, 60)}
      options={[
        {
          title: metadata,
          value: "__meta__",
          category: "Memory",
        },
        ...contentLines.map((line: string, index: number) => ({
          title: line || " ",
          value: `__line__${index}`,
        })),
        { title: "✕ Delete Memory", value: "__delete__", category: NAV_CATEGORY },
        { title: "← Back", value: "__back__", category: NAV_CATEGORY },
      ]}
      onSelect={(opt: any) => {
        if (opt.value === "__back__") showProjectMemoriesMenuFn(api);
        else if (opt.value === "__delete__") showDeleteMemory(api, memory);
        else showMemoryDetail(api, memory);
      }}
      onCancel={() => showProjectMemoriesMenuFn(api)}
    />
  ));
}

function showDeleteMemory(api: any, memory: any) {
  const title = memory.title || memory.topic_key || `Memory #${memory.id}`;

  api.ui.dialog.replace(() => (
    <api.ui.DialogConfirm
      title="Delete Memory"
      message={`Eliminar '${truncateText(title, 48)}'?`}
      onConfirm={() => {
        try {
          deleteProjectMemory(memory.id);
          api.ui.toast({ title: "Deleted", message: "Memory eliminada", variant: "success" });
          showProjectMemoriesMenuFn(api);
        } catch (e: any) {
          api.ui.toast({ title: "Error", message: e.message || "No se pudo eliminar la memory", variant: "error" });
          showMemoryDetail(api, memory);
        }
      }}
      onCancel={() => showMemoryDetail(api, memory)}
    />
  ));
}

// Referencias a funciones de dialog para evitar dependencias circulares
let showProfilesMenuFn: (api: any) => void;
let showProfileListFn: (api: any) => void;
let showProfileDetailFn: (api: any, profileOpt: any) => void;
let showProjectMemoriesMenuFn: (api: any) => void;

export function registerDialogCallbacks(callbacks: {
  showProfilesMenu: (api: any) => void;
  showProfileList: (api: any) => void;
  showProfileDetail: (api: any, profileOpt: any) => void;
  showProjectMemoriesMenu: (api: any) => void;
}) {
  showProfilesMenuFn = callbacks.showProfilesMenu;
  showProfileListFn = callbacks.showProfileList;
  showProfileDetailFn = callbacks.showProfileDetail;
  showProjectMemoriesMenuFn = callbacks.showProjectMemoriesMenu;
}

export function showProfilesMenu(api: any) {
  api.ui.dialog.replace(() => (
    <api.ui.DialogSelect
      title="SDD Profile Management"
      options={[
        {
          title: "󰏪 Create New SDD Profile",
          value: "create",
          description: "Save current config as 'name.json'.",
        },
        {
          title: "󰓅 Manage SDD Profiles",
          value: "list",
          description: "List and activate saved SDD profiles.",
        },
        {
          title: "󰄄 View Project Memories",
          value: "view_memories",
          description: "Show recent Engram memories for this project.",
        },
        {
          title: "✕ Close",
          value: "__close__",
          category: NAV_CATEGORY,
        },
      ]}
      onSelect={(opt: any) => {
        if (opt.value === "create") showCreateProfile(api);
        else if (opt.value === "list") showProfileListFn(api);
        else if (opt.value === "view_memories") showProjectMemoriesMenuFn(api);
        else api.ui.dialog.clear();
      }}
      onCancel={() => api.ui.dialog.clear()}
    />
  ));
}

export function showCreateProfile(api: any) {
  const { configPath, profilesDir } = resolvePaths();
  ensureProfilesDir();

  api.ui.dialog.replace(() => (
    <api.ui.DialogPrompt
      title="New SDD Profile Name"
      placeholder="Enter profile name"
      onConfirm={(name: string) => {
        const trimmed = name?.trim();
        if (!trimmed) {
          showProfilesMenuFn(api);
          return;
        }

        const finalName = trimmed.replace(/\.json$/i, "");
        const fileName = `${finalName}.json`;
        const profilePath = path.join(profilesDir, fileName);

        if (fs.existsSync(profilePath)) {
          api.ui.toast({
            title: "Error",
            message: `Profile '${finalName}' already exists`,
            variant: "error",
          });
          showProfilesMenuFn(api);
          return;
        }

        try {
          const currentConfig = JSON.parse(fs.readFileSync(configPath, "utf-8"));
          const profileModels = extractSddAgentModels(currentConfig);

          if (Object.keys(profileModels).length === 0) {
            api.ui.toast({
              title: "Error",
              message: "No encontré agentes SDD con modelo asignado",
              variant: "error",
            });
            showProfilesMenuFn(api);
            return;
          }

          writeProfileModels(profilePath, profileModels);
          api.ui.toast({
            title: "Success",
            message: `Profile '${finalName}' created`,
            variant: "success",
          });
          showProfilesMenuFn(api);
        } catch (e: any) {
          api.ui.toast({
            title: "Error",
            message: `Failed to create profile: ${e.message}`,
            variant: "error",
          });
          showProfilesMenuFn(api);
        }
      }}
      onCancel={() => showProfilesMenuFn(api)}
    />
  ));
}

export function showProfileList(api: any) {
  ensureProfilesDir();

  const files = listProfileFiles();

  if (files.length === 0) {
    api.ui.toast({
      title: "No Profiles",
      message: "No profiles starting with 'sdd-' found.",
      variant: "warning",
    });
    showProfilesMenuFn(api);
    return;
  }

  const activeFile = detectActiveProfileFile(files, api);

  api.ui.dialog.replace(() => (
    <api.ui.DialogSelect
      title="Select SDD Profile"
      current={activeFile}
      options={[
        ...files.map((f) => ({
          title: `${f === activeFile ? "✓ " : ""}${f.replace(".json", "")}`,
          value: f,
          description: f === activeFile ? "✓ Active" : "SDD Profile",
        })),
        { title: "← Back", value: "__back__", category: NAV_CATEGORY },
      ]}
      onSelect={(opt: any) => {
        if (opt.value === "__back__") showProfilesMenuFn(api);
        else showProfileDetailFn(api, { title: String(opt.value).replace(".json", ""), value: opt.value });
      }}
      onCancel={() => showProfilesMenuFn(api)}
    />
  ));
}

export function showProfileDetail(api: any, profileOpt: any) {
  const { profilesDir } = resolvePaths();
  try {
    const profilePath = path.join(profilesDir, profileOpt.value);
    const profileModels = readProfileModels(profilePath);
    const sddAgents = Object.entries(profileModels);

    api.ui.dialog.replace(() => (
      <api.ui.DialogSelect
        title={`Profile: ${profileOpt.title}`}
        options={[
          { title: `✏ Name: ${profileOpt.title}`, value: "__rename__", category: "Profile" },
          ...sddAgents.map(([name, modelId]) => ({
            title: name,
            value: name,
            description: resolveModelInfo(api, modelId),
            category: "Agents (Click to edit model)",
          })),
          { title: "✓ Activate Profile", value: "__assign__", category: NAV_CATEGORY },
          { title: "✕ Delete Profile", value: "__delete__", category: NAV_CATEGORY },
          { title: "← Back", value: "__back__", category: NAV_CATEGORY },
        ]}
        onSelect={(opt: any) => {
          if (opt.value === "__back__") showProfileListFn(api);
          else if (opt.value === "__assign__") handleActivateProfile(api, profilePath, profileOpt.title);
          else if (opt.value === "__delete__") showDeleteProfile(api, profileOpt);
          else if (opt.value === "__rename__") showRenameProfile(api, profileOpt);
          else if (!opt.value.startsWith("__")) showProviderPickerForAgent(api, profileOpt, opt.value);
        }}
        onCancel={() => showProfileListFn(api)}
      />
    ));
  } catch (e) {
    api.ui.toast({ title: "Error", message: "Failed to read profile details", variant: "error" });
  }
}

async function handleActivateProfile(api: any, profilePath: string, profileName: string) {
  const updatedConfig = await activateProfileFile(api, profilePath, profileName);
  if (!updatedConfig) return;

  setActiveProfile(parseActiveProfileFromRaw(JSON.stringify(updatedConfig), api));

  api.ui.dialog.replace(() => (
    <api.ui.DialogConfirm
      title="Profile Activated"
      message={`Perfil '${profileName}' aplicado sobre la config global. Probando recarga en runtime.`}
      onConfirm={() => api.ui.dialog.clear()}
      onCancel={() => api.ui.dialog.clear()}
    />
  ));
}

function showDeleteProfile(api: any, profileOpt: any) {
  api.ui.dialog.replace(() => (
    <api.ui.DialogConfirm
      title="Delete Profile"
      message={`Permanently delete '${profileOpt.title}'?`}
      onConfirm={() => {
        try {
          deleteProfileFile(profileOpt.value);
          api.ui.toast({ title: "Deleted", message: `Profile '${profileOpt.title}' deleted` });
          showProfileListFn(api);
        } catch (e: any) {
          api.ui.toast({ title: "Error", message: `Failed to delete: ${e.message}`, variant: "error" });
          showProfileDetailFn(api, profileOpt);
        }
      }}
      onCancel={() => showProfileDetailFn(api, profileOpt)}
    />
  ));
}

function showRenameProfile(api: any, profileOpt: any) {
  api.ui.dialog.replace(() => (
    <api.ui.DialogPrompt
      title="Rename Profile"
      value={profileOpt.title}
      onConfirm={(newName: string) => {
        const trimmed = newName?.trim();
        if (!trimmed || trimmed === profileOpt.title) {
          showProfileDetailFn(api, profileOpt);
          return;
        }

        const finalName = trimmed.replace(/\.json$/i, "");
        const newFileName = `${finalName}.json`;

        const { profilesDir } = resolvePaths();
        const newPath = path.join(profilesDir, newFileName);

        if (fs.existsSync(newPath)) {
          api.ui.toast({ title: "Error", message: "Name already exists", variant: "error" });
          showProfileDetailFn(api, profileOpt);
          return;
        }

        try {
          renameProfileFile(profileOpt.value, newFileName);
          api.ui.toast({ title: "Renamed", message: `Profile renamed to '${finalName}'` });
          showProfileListFn(api);
        } catch (e: any) {
          api.ui.toast({ title: "Error", message: `Failed to rename: ${e.message}`, variant: "error" });
          showProfileDetailFn(api, profileOpt);
        }
      }}
      onCancel={() => showProfileDetailFn(api, profileOpt)}
    />
  ));
}

function showProviderPickerForAgent(api: any, profileOpt: any, agentName: string) {
  const providers = (api.state.provider || []).filter((p: any) => Object.keys(p.models || {}).length > 0);

  if (providers.length === 0) {
    api.ui.toast({ title: "No Providers", message: "No authenticated providers found.", variant: "warning" });
    showProfileDetailFn(api, profileOpt);
    return;
  }

  api.ui.dialog.replace(() => (
    <api.ui.DialogSelect
      title={`Provider for ${agentName}`}
      options={[
        ...providers.map((p: any) => ({
          title: p.name || p.id,
          value: p.id,
          description: `${Object.keys(p.models || {}).length} models available`,
        })),
        { title: "← Back", value: "__back__", category: NAV_CATEGORY },
      ]}
      onSelect={(opt: any) => {
        if (opt.value === "__back__") showProfileDetailFn(api, profileOpt);
        else {
          const selected = providers.find((p: any) => p.id === opt.value);
          showModelPickerForAgent(api, profileOpt, agentName, selected);
        }
      }}
      onCancel={() => showProfileDetailFn(api, profileOpt)}
    />
  ));
}

function showModelPickerForAgent(api: any, profileOpt: any, agentName: string, provider: any) {
  const models = provider.models || {};
  const modelKeys = Object.keys(models);

  api.ui.dialog.replace(() => (
    <api.ui.DialogSelect
      title={`${provider.name || provider.id} › ${agentName}`}
      options={[
        ...modelKeys.map((key) => {
          const model = models[key];
          const ctxText = model.limit?.context ? formatContext(model.limit.context) : "ctx: N/A";
          return {
            title: model.name || key,
            value: `${provider.id}/${key}`,
            description: ctxText,
          };
        }),
        { title: "← Back", value: "__back__", category: NAV_CATEGORY },
      ]}
      onSelect={(opt: any) => {
        if (opt.value === "__back__") showProviderPickerForAgent(api, profileOpt, agentName);
        else updateAgentModel(api, profileOpt, agentName, opt.value);
      }}
      onCancel={() => showProviderPickerForAgent(api, profileOpt, agentName)}
    />
  ));
}

function updateAgentModel(api: any, profileOpt: any, agentName: string, fullModelId: string) {
  const { profilesDir } = resolvePaths();
  const profilePath = path.join(profilesDir, profileOpt.value);

  try {
    const profileModels = readProfileModels(profilePath);
    profileModels[agentName] = fullModelId;
    writeProfileModels(profilePath, profileModels);
    api.ui.toast({ title: "Updated", message: `${agentName} set to ${fullModelId}`, variant: "success" });
    showProfileDetailFn(api, profileOpt);
  } catch (e: any) {
    api.ui.toast({ title: "Error", message: `Failed to update agent: ${e.message}`, variant: "error" });
    showProfileDetailFn(api, profileOpt);
  }
}

export function showProjectMemoriesMenu(api: any) {
  const projectName = api?.state?.path?.directory ? path.basename(api.state.path.directory) : "project";

  try {
    const memories = listProjectMemories(api);

    if (memories.length === 0) {
      api.ui.toast({
        title: "No Memories",
        message: `No engram memories for ${projectName}`,
        variant: "warning",
      });
      showProfilesMenuFn(api);
      return;
    }

    api.ui.dialog.replace(() => (
      <api.ui.DialogSelect
        title={`Memories: ${projectName}`}
        options={[
          ...memories.map((m) => ({
            title: truncateText(`[${m.id}] ${m.title || m.topic_key || `Memory #${m.id}`}`, 60),
            value: String(m.id),
            description: `[${(m.type || "manual").toUpperCase()}] ${formatMemoryDate(
              m.updated_at || m.created_at
            )} · ${m.scope || "project"}`,
          })),
          { title: "← Back", value: "__back__", category: NAV_CATEGORY },
        ]}
        onSelect={(opt: any) => {
          if (opt.value === "__back__") showProfilesMenuFn(api);
          else {
            const memory = memories.find((item) => String(item.id) === opt.value);
            if (!memory) return;
            showMemoryDetail(api, memory);
          }
        }}
        onCancel={() => showProfilesMenuFn(api)}
      />
    ));
  } catch (e) {
    api.ui.toast({
      title: "Engram Error",
      message: "Failed to read local project memories.",
      variant: "error",
    });
    showProfilesMenuFn(api);
  }
}
