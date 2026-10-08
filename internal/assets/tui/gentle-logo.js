// gentle-logo-tui.tsx
import { createComponent as _$createComponent } from "@opentui/solid";
import { memo as _$memo } from "@opentui/solid";
import { effect as _$effect } from "@opentui/solid";
import { insert as _$insert } from "@opentui/solid";
import { setProp as _$setProp } from "@opentui/solid";
import { createElement as _$createElement } from "@opentui/solid";
import { useTerminalDimensions } from "@opentui/solid";
import { createMemo, createRoot } from "solid-js";
var id = "gentle-logo";
var roseArt = ["             \u28E0\u28FE\u28F7\u28F6\u28E6\u28E4\u28E4\u28C4\u28E0\u28C4\u28C0  \u2880\u28C0\u28C0", "          \u2880\u28F4\u28FF\u28FF\u283F\u28CB\u28ED\u28ED\u28EF\u28ED\u28CD\u28ED\u28FF\u28DF\u281B\u281B\u283F\u283F\u28FF\u28F7\u28C4", "      \u2880\u28F4\u28FE\u285F\u28BB\u28FF\u285F\u2801\u28FC\u28FF\u280F\u28F5\u28BB\u28FF\u28FB\u28FF\u28FF\u28BF\u287B\u28FF\u28FF\u28F6\u284C\u28BF\u28FF\u28F7\u28E6\u28E4\u2844", "   \u28E4\u28F6\u28FE\u28FF\u28FF\u280F \u2808\u28BF\u28C4 \u28B9\u28CF\u2820\u281F\u28FE\u28FF\u28FF\u28FF\u28FF\u28FF\u2837\u28CF\u28FC\u281F\u28A1\u28FF\u285F\u280B\u28BB\u28FF\u28FF\u2844", "   \u2808\u28FF\u28FF\u28FF\u28FF\u2846   \u28FD\u28A7\u2858\u2808\u2833\u28E6\u28CD\u281B\u281B\u28A6\u28C9\u28F4\u28DB\u28EB\u28ED\u28F4\u285F\u280B  \u28FE\u28FF\u28FF\u287F", "   \u2880\u2839\u28FF\u28FF\u28FF\u28F7\u28E4\u2844 \u280B \u2819\u2886 \u28E0\u2834\u281F\u281B\u28DB\u28DB\u28DB\u281F\u280B\u2801\u283A\u2847 \u28C0\u28F4\u28FF\u28FF\u285F\u2801", "   \u2808\u28C0\u2808\u281B\u2837\u283F\u28FF\u28FF\u28F7\u28E4\u28C0 \u28A0\u280B   \u2808\u2809\u2809    \u28E0\u28F4\u28E5\u283E\u281B\u2809\u28F0\u28FF\u28F7", "          \u2839\u28EF\u28DD\u281B\u281B\u2837\u28B6\u28E4\u28E4\u28C0   \u2880\u2860\u2816\u280B\u2809\u2889\u28C0\u28C0\u28F4\u28FE\u28FF\u283F\u281F\u2803 \u2820\u2826", "\u2801       \u2816  \u2818\u283B\u28BF\u28E6\u28C4\u2840  \u2809\u281B\u28A6\u2820\u288A\u2824\u2834\u2892\u28DB\u28DB\u28E9\u28FD\u287F\u281F\u2801\u2880\u2840", "\u2832\u2836\u28E6\u2834\u2836\u2836\u2836\u2836\u2876\u2836\u28B6\u28E4\u28C4\u2840\u2828\u282D\u283D\u281F\u28D3\u28A6\u28C0\u2808\u2887\u2865\u2816\u281B\u280B\u2809\u2809\u2809    \u2808  \u28A0\u2864", "  \u2808\u28B7 \u2810\u2802\u28A4\u28FD\u28C4 \u2830\u284E\u2819\u2833\u28C4\u2840 \u2808\u28A3\u2818\u28A6\u280B\u28C0\u286C\u281F\u281B\u281B\u2809\u2880\u28C0\u28C0\u28E0\u2864\u2804\u2803", "   \u2808\u28B3\u28C0\u2852\u2809\u2809\u28C9\u2819\u2872\u28FD\u28C4 \u28CF\u2833\u2844 \u2818\u2847 \u287E\u2801 \u2880\u2864\u2816\u28FB\u28FF\u284F\u28A1\u284E \u2830\u2804", "     \u281B\u283B\u28A6\u28C4\u28C9\u2841\u28C0\u28C0\u28C8\u28D9\u28FA\u28CC\u2847\u28A0\u2880\u2847\u287E  \u28F4\u28FF\u2877\u280A \u28B2\u28E0\u281F", "          \u2808\u2809    \u2808\u2833\u2844\u28F8\u28B1\u2807\u2880\u28F0\u28EF\u28ED\u28E5\u282D\u283E\u281B\u2803", "                  \u2877\u2821\u286F\u2896\u2809   \u28A0\u2824", "                \u2860\u288A\u2874\u2824\u2802\u2803 \u2812", "             \u2880\u2874\u28AA\u2814\u28C9\u2814\u280B", "               \u2810\u2808"];
var compactArt = ["\u2726 Gentle AI \u2726"];
var resolveLogoColor = (theme) => {
  const current = theme?.current ?? theme;
  const candidate = current?.logoColor ?? current?.accent ?? current?.magenta;
  return typeof candidate === "string" && candidate.length > 0 ? candidate : "magenta";
};
var Logo = (props) => {
  const dim = useTerminalDimensions();
  const lines = createMemo(() => {
    const term = dim();
    return term.height >= roseArt.length + 6 && term.width >= 64 ? roseArt : compactArt;
  });
  return (() => {
    var _el$ = _$createElement("box");
    _$setProp(_el$, "flexDirection", "column");
    _$setProp(_el$, "alignItems", "center");
    _$insert(_el$, () => lines().map((line) => (() => {
      var _el$2 = _$createElement("text");
      _$insert(_el$2, line);
      _$effect((_$p) => _$setProp(_el$2, "fg", resolveLogoColor(props.theme), _$p));
      return _el$2;
    })()));
    return _el$;
  })();
};
var logoFor = (api, ctx) => _$createComponent(Logo, {
  get theme() {
    return ctx?.theme?.current || ctx?.theme || api.theme;
  }
});
var initialize = (api) => {
  if (api.ui && typeof api.ui.slot === "function") {
    api.ui.slot({
      prepend: "home.footer",
      render: (ctx) => logoFor(api, ctx)
    });
  }
  if (api.slots && typeof api.slots.register === "function") {
    api.slots.register({
      order: 100,
      slots: {
        home_logo: (ctx) => logoFor(api, ctx)
      }
    });
  }
};
var Plugin = {
  define: (def) => {
    if (!def.server && def.setup) {
      def.server = def.setup;
    }
    return def;
  }
};
var tui = async (api) => {
  createRoot(() => initialize(api));
};
var setup = (context) => createRoot(() => initialize(context));
var GentleLogoPluginDefinition = Plugin.define({
  id,
  setup,
  tui
});
var gentle_logo_tui_default = GentleLogoPluginDefinition;
export {
  GentleLogoPluginDefinition,
  Plugin,
  gentle_logo_tui_default as default
};
