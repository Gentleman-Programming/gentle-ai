/** @jsxImportSource @opentui/solid */
// @ts-nocheck

export function ActiveModelBadge(props: { profile: any; theme: any }) {
  const formatContext = (tokens: number | null): string => {
    if (!tokens || typeof tokens !== "number") return "ctx: N/A";
    if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1).replace(/\.0$/, "")}M ctx`;
    if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}k ctx`;
    return `${tokens} ctx`;
  };

  const text = props.profile 
    ? `${props.profile.modelName} · ${formatContext(props.profile.contextLimit)}` 
    : "No SDD model active";

  const color = props.profile 
    ? (props.theme?.primary || "#00ff00") 
    : (props.theme?.textMuted || "#888");

  return (
    <box flexDirection="row" alignItems="center" padding={{ left: 1, right: 1 }}>
      <text fg={color} bold={props.profile ? true : false}>
        {props.profile ? "󰚩 " : "󱚧 "}
      </text>
      <text fg={props.theme?.text || "inherit"}>
        {text}
      </text>
    </box>
  );
}
