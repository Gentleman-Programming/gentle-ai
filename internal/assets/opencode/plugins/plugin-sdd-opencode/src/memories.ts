/** @jsxImportSource @opentui/solid */
// @ts-nocheck

/**
 * Lógica de memorias Engram - Lee directamente de SQLite
 */

import { execFileSync } from "node:child_process";
import * as path from "node:path";
import * as os from "node:os";
import { resolveProjectCandidates, resolveProjectName } from "./config";
import type { EngramObservation } from "./types";

const ENGRAM_DB = path.join(os.homedir(), ".engram", "engram.db");

function execSQLiteJson(query: string): EngramObservation[] {
  try {
    const output = execFileSync(
      "sqlite3",
      ["-json", ENGRAM_DB, query],
      {
        encoding: "utf-8",
        timeout: 5000,
        stdio: ["ignore", "pipe", "ignore"],
      }
    );

    if (!output.trim()) return [];

    return JSON.parse(output) as EngramObservation[];
  } catch {
    return [];
  }
}

function execSQLite(query: string): void {
  execFileSync("sqlite3", [ENGRAM_DB, query], {
    encoding: "utf-8",
    timeout: 5000,
    stdio: ["ignore", "pipe", "ignore"],
  });
}

export function listProjectMemories(api: any): EngramObservation[] {
  const projectName = resolveProjectName(api);
  const projectCandidates = resolveProjectCandidates(api);
  const escapedProjects = projectCandidates
    .map((name) => name.replace(/'/g, "''"))
    .map((name) => `'${name}'`)
    .join(", ");

  if (!escapedProjects) return [];

  const query = `
    SELECT
      id,
      type,
      title,
      content,
      project,
      scope,
      created_at,
      updated_at,
      ifnull(topic_key, '') as topic_key
    FROM observations
    WHERE project IN (${escapedProjects})
      AND deleted_at IS NULL
    ORDER BY updated_at DESC
  `;

  return execSQLiteJson(query).map((memory) => ({
    ...memory,
    project: memory.project || projectName,
    scope: memory.scope || "project",
  }));
}

export function deleteProjectMemory(memoryId: number): void {
  const safeId = Number(memoryId);
  if (!Number.isInteger(safeId) || safeId <= 0) {
    throw new Error("Memory ID inválido");
  }

  execSQLite(`
    UPDATE observations
    SET deleted_at = datetime('now'), updated_at = datetime('now')
    WHERE id = ${safeId}
      AND deleted_at IS NULL;
  `);
}
