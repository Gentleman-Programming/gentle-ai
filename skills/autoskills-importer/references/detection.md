# Detection Reference — Stack Detection

## Goal

Produce a structured list of detected technologies (name, version, confidence)
from project files. This list drives which skills to fetch from skills.sh.
Only high and medium confidence detections trigger a fetch.

---

## File Priority

Read in this order. Earlier signals override later ones when they conflict.

1. Lock files — determine package manager
2. Framework config files — strongest signal for framework + version
3. `package.json` dependencies — secondary signal
4. `package.json` devDependencies — weakest signal; alone = low confidence

Signal strength: config file > `dependencies` > `devDependencies`

---

## Files to Check

### Package manager
- `package-lock.json` → npm
- `yarn.lock` → yarn
- `pnpm-lock.yaml` → pnpm
- `bun.lockb` → bun

### Framework config files (high confidence)
- `next.config.{js,ts,mjs}` → nextjs
- `vite.config.{js,ts}` → vite
- `astro.config.{mjs,ts}` → astro
- `nuxt.config.{ts,js}` → nuxt
- `svelte.config.js` → sveltekit
- `remix.config.{js,cjs}` → remix
- `angular.json` → angular

### Styling
- `tailwind.config.{js,ts,cjs}` → tailwind
- `postcss.config.{js,cjs}` → postcss

### Testing
- `vitest.config.{js,ts}` → vitest
- `jest.config.{js,ts,cjs}` → jest
- `playwright.config.{ts,js}` → playwright
- `cypress.config.{ts,js}` → cypress

### TypeScript
- `tsconfig.json` → typescript

---

## Version Extraction

Normalize to `major.minor` from package.json. Examples:

- `"react": "^19.0.0"` → `19.0`
- `"react": "~18.2.0"` → `18.2`
- `"react": "workspace:*"` → check lock file

Version matters when skills.sh has version-specific skills (e.g., react-18
vs react-19, nextjs-14 vs nextjs-15). When it doesn't, `"any"` is acceptable.

If version cannot be resolved, mark as `unknown` and treat as medium confidence.

---

## Confidence Rules

| Signal | Confidence |
|---|---|
| Config file found | high |
| `dependencies` with explicit version | high |
| `dependencies` with `*` or `latest` | medium |
| `devDependencies` only, no config file | low |

Fetch skills for `high` and `medium`. For `low`, list them separately and
ask the user to confirm before including in the fetch list.

---

## Monorepo Handling

If `package.json` contains `"workspaces"`, also check each workspace root.
Merge detections — a technology found in any workspace counts as detected.
Record the workspace path in the `signal` field.

---

## Skill Name Mapping

Map detected technology + version to a skills.sh key. Validate each key
against the live index before fetching (Step 2 in SKILL.md).

| Detected | skills.sh key |
|---|---|
| react@19.x | react-19 |
| react@18.x | react-18 |
| nextjs@15.x | nextjs-15 |
| nextjs@14.x | nextjs-14 |
| tailwind@4.x | tailwind-v4 |
| tailwind@3.x | tailwind-v3 |
| vitest (any) | vitest |
| typescript (any) | typescript |

If no mapping exists for a detected technology, skip it and log it as a gap.
Do not guess or approximate a key — missing mappings belong in this table.

---

## Output Schema

```yaml
package_manager: npm | yarn | pnpm | bun | unknown
technologies:
  - name: react
    version: "19.0"
    signal: package.json#dependencies
    confidence: high
  - name: nextjs
    version: "15.1"
    signal: next.config.ts
    confidence: high
low_confidence:
  - name: jest
    signal: devDependencies only
pending_user_confirmation: true   # true if low_confidence is non-empty
```
