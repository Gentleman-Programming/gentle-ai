// gentle-logo TUI plugin source. Built with tsup + esbuild-plugin-solid into
// ../assets/tui/gentle-logo.js (committed, go:embedded); solid-js, @opentui/*,
// and @opencode-ai/plugin stay external because the OpenCode host provides
// them. OpenCode never transpiles raw TSX, so this pre-built bundle is the
// only artifact the installer ships (issue #5364).
import type { TuiPlugin } from "@opencode-ai/plugin/tui";
import { useTerminalDimensions } from "@opentui/solid";
import { createMemo, createRoot } from "solid-js";

const id = "gentle-logo";

const roseArt = [
  "             ⣠⣾⣷⣶⣦⣤⣤⣄⣠⣄⣀  ⢀⣀⣀",
  "          ⢀⣴⣿⣿⠿⣋⣭⣭⣯⣭⣍⣭⣿⣟⠛⠛⠿⠿⣿⣷⣄",
  "      ⢀⣴⣾⡟⢻⣿⡟⠁⣼⣿⠏⣵⢻⣿⣻⣿⣿⢿⡻⣿⣿⣶⡌⢿⣿⣷⣦⣤⡄",
  "   ⣤⣶⣾⣿⣿⠏ ⠈⢿⣄ ⢹⣏⠠⠟⣾⣿⣿⣿⣿⣿⠷⣏⣼⠟⢡⣿⡟⠋⢻⣿⣿⡄",
  "   ⠈⣿⣿⣿⣿⡆   ⣽⢧⡘⠈⠳⣦⣍⠛⠛⢦⣉⣴⣛⣫⣭⣴⡟⠋  ⣾⣿⣿⡿",
  "   ⢀⠹⣿⣿⣿⣷⣤⡄ ⠋ ⠙⢆ ⣠⠴⠟⠛⣛⣛⣛⠟⠋⠁⠺⡇ ⣀⣴⣿⣿⡟⠁",
  "   ⠈⣀⠈⠛⠷⠿⣿⣿⣷⣤⣀ ⢠⠋   ⠈⠉⠉    ⣠⣴⣥⠾⠛⠉⣰⣿⣷",
  "          ⠹⣯⣝⠛⠛⠷⢶⣤⣤⣀   ⢀⡠⠖⠋⠉⢉⣀⣀⣴⣾⣿⠿⠟⠃ ⠠⠦",
  "⠁       ⠖  ⠘⠻⢿⣦⣄⡀  ⠉⠛⢦⠠⢊⠤⠴⢒⣛⣛⣩⣽⡿⠟⠁⢀⡀",
  "⠲⠶⣦⠴⠶⠶⠶⠶⡶⠶⢶⣤⣄⡀⠨⠭⠽⠟⣓⢦⣀⠈⢇⡥⠖⠛⠋⠉⠉⠉    ⠈  ⢠⡤",
  "  ⠈⢷ ⠐⠂⢤⣽⣄ ⠰⡎⠙⠳⣄⡀ ⠈⢣⠘⢦⠋⣀⡬⠟⠛⠛⠉⢀⣀⣀⣠⡤⠄⠃",
  "   ⠈⢳⣀⡒⠉⠉⣉⠙⡲⣽⣄ ⣏⠳⡄ ⠘⡇ ⡾⠁ ⢀⡤⠖⣻⣿⡏⢡⡎ ⠰⠄",
  "     ⠛⠻⢦⣄⣉⡁⣀⣀⣈⣙⣺⣌⡇⢠⢀⡇⡾  ⣴⣿⡷⠊ ⢲⣠⠟",
  "          ⠈⠉    ⠈⠳⡄⣸⢱⠇⢀⣰⣯⣭⣥⠭⠾⠛⠃",
  "                  ⡷⠡⡯⢖⠉   ⢠⠤",
  "                ⡠⢊⡴⠤⠂⠃ ⠒",
  "             ⢀⡴⢪⠔⣉⠔⠋",
  "               ⠐⠈",
];

const compactArt = ["✦ Gentle AI ✦"];

// resolveLogoColor prefers the live theme palette the host passes through the
// slot context and falls back to the built-in magenta when the theme shape is
// unrecognized, so an unexpected theme object can never break rendering.
const resolveLogoColor = (theme: any): string => {
  const current = theme?.current ?? theme;
  const candidate = current?.logoColor ?? current?.accent ?? current?.magenta;
  return typeof candidate === "string" && candidate.length > 0 ? candidate : "magenta";
};

const Logo = (props: { theme?: any }) => {
  const dim = useTerminalDimensions();
  const lines = createMemo(() => {
    const term = dim();
    return term.height >= roseArt.length + 6 && term.width >= 64 ? roseArt : compactArt;
  });

  return (
    <box flexDirection="column" alignItems="center">
      {lines().map((line) => (
        <text fg={resolveLogoColor(props.theme)}>{line}</text>
      ))}
    </box>
  );
};

const logoFor = (api: any, ctx: any) => (
  <Logo theme={ctx?.theme?.current || ctx?.theme || api.theme} />
);

const initialize = (api: any) => {
  // OpenCode v2 has no v1 home_logo slot; the banner prepends the home footer
  // surface instead.
  if (api.ui && typeof api.ui.slot === "function") {
    api.ui.slot({
      prepend: "home.footer",
      render: (ctx: any) => logoFor(api, ctx),
    });
  }

  // OpenCode v1 slot registration.
  if (api.slots && typeof api.slots.register === "function") {
    api.slots.register({
      order: 100,
      slots: {
        home_logo: (ctx: any) => logoFor(api, ctx),
      },
    });
  }
};

// OpenCode v2 validates plugin modules against the server-side hook contract,
// so expose the optional setup entry as the server hook when the module does
// not define one of its own.
export const Plugin = {
  define: <T extends { id: string; setup?: (ctx: any) => any; tui?: (api: any) => any; server?: (ctx: any) => any }>(def: T): T => {
    if (!def.server && def.setup) {
      def.server = def.setup;
    }
    return def;
  },
};

const tui: TuiPlugin = async (api: any) => {
  createRoot(() => initialize(api));
};

const setup = (context: any) => createRoot(() => initialize(context));

export const GentleLogoPluginDefinition = Plugin.define({
  id,
  setup,
  tui,
});

export default GentleLogoPluginDefinition;
