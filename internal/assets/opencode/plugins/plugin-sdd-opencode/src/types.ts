/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Tipos compartidos del plugin SDD Model Select
 */

export type ActiveProfileState = {
  modelId: string;
  contextLimit: number | null;
  providerName: string;
  modelName: string;
};

export type ProfileModels = Record<string, string>;

export type ProfileState = {
  activeProfile?: string;
  updatedAt?: string;
};

export type EngramObservation = {
  id: number;
  type: string;
  title?: string;
  topic_key?: string;
  content?: string;
  project: string;
  scope?: string;
  updated_at?: string;
  created_at?: string;
};

export type ProfileOption = {
  title: string;
  value: string;
};

export const NAV_CATEGORY = "─────────────";
