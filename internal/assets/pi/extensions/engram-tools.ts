// gentle-ai managed template for pi
// This file documents the expected Engram bridge entrypoint for pi.
// Replace with your package implementation if you maintain a custom bridge.

export default function registerEngramTools() {
  return {
    backend: "engram-mcp",
    tools: ["mem_save", "mem_search", "mem_update", "mem_delete"],
  };
}
