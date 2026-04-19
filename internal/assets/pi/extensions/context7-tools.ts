// gentle-ai managed template for pi
// This file documents the expected Context7 bridge entrypoint for pi.
// Replace with your package implementation if you maintain a custom bridge.

export default function registerContext7Tools() {
  return {
    backend: "context7-mcp",
    tools: ["resolve-library-id", "get-library-docs", "query-docs"],
  };
}
