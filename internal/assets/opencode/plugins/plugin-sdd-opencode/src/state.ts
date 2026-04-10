/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Estado global del plugin
 */

import { ActiveProfileState } from "./types";

export let activeProfile: ActiveProfileState | null = null;

export function setActiveProfile(profile: ActiveProfileState | null): void {
  activeProfile = profile;
}
