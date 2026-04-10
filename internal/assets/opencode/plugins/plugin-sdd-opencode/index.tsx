/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Entry point del plugin SDD Model Select
 */

import type { TuiPlugin, TuiPluginModule } from "@opencode-ai/plugin";
import * as fs from "node:fs";
import { ActiveModelBadge } from "./components";

// Importaciones directas (sin barrel export para evitar problemas de resolución)
import { activeProfile, setActiveProfile } from "./src/state";
import { resolvePaths } from "./src/config";
import { parseActiveProfileFromRaw } from "./src/utils";
import {
  showProfilesMenu,
  showProfileList,
  showProfileDetail,
  showProjectMemoriesMenu,
  registerDialogCallbacks,
} from "./src/dialogs";

// -- Plugin Initialization ---------------------------------------------------

function initializeDialogs() {
  // Registra los callbacks para resolver dependencias circulares entre dialogs
  registerDialogCallbacks({
    showProfilesMenu,
    showProfileList,
    showProfileDetail,
    showProjectMemoriesMenu,
  });
}

function readActiveProfile(api: any) {
  const { configPath } = resolvePaths();
  try {
    if (!fs.existsSync(configPath)) return null;
    const raw = fs.readFileSync(configPath, "utf-8");
    return parseActiveProfileFromRaw(raw, api);
  } catch {
    return null;
  }
}

// -- Plugin Entry ------------------------------------------------------------

const id = "sdd-model-select";

const tui: TuiPlugin = async (api) => {
  // Inicializar callbacks de dialogs
  initializeDialogs();

  // Leer y establecer perfil activo en estado global
  const profile = readActiveProfile(api);
  setActiveProfile(profile);

  // Registrar comando
  api.command.register(() => [
    {
      title: "󰓅 SDD Profiles",
      value: "sdd-profiles",
      keybind: "alt+k",
      slash: { name: "sdd-model" },
      onSelect: () => showProfilesMenu(api),
    },
  ]);

  // Registrar slots UI - usan estado global directamente
  api.slots.register({
    slots: {
      home_bottom(ctx: any) {
        return <ActiveModelBadge profile={activeProfile} theme={ctx.theme.current} />;
      },
      sidebar_content(ctx: any) {
        return <ActiveModelBadge profile={activeProfile} theme={ctx.theme.current} />;
      },
    },
  });
};

const plugin: TuiPluginModule & { id: string } = { id, tui };
export default plugin;
