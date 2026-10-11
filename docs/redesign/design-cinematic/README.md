# momoshtrem · Cinematic dark (exploration A)

The output of the Cinematic dark design brief in `../design-prompt-cinematic.md` (PLAN.md task D1, exploration A).

Open `index.html` in a browser. It works straight from the file system and needs network access only for Google Fonts. From there:

| Page | What's on it |
|---|---|
| **Flows** | The three clickable prototype flows, in an inline player. |
| **`system.html`** | Logo explorations and final set, Noir the mascot, tokens, and every component with its states. |
| **`screens.html`** | All 13 screens and their states at 390 / 768 / 1440. |
| **`prototype.html`** | The full prototype. Each screen has a dashed violet "State" switcher. |
| **`HANDOFF.md`** | Layout grid, component inventory, interaction details, and the data the screens need beyond PLAN.md §6. |

Deliverables map:

| Brief deliverable | Where |
|---|---|
| 3 logo explorations + final set | `assets/logo/`, `system.html#logo` |
| Design system page | `system.html` |
| Every screen at 390 / 1440 (+768 for key screens) with states | `screens.html`, PNG exports in `screens/` |
| Clickable prototype of 3 flows | `index.html#flows` |
| Tokens (CSS custom properties) + Tailwind v4 `@theme` | `css/tokens.css`, `tokens/theme.css` |
| Handoff notes | `HANDOFF.md` |

To regenerate the PNGs:

```bash
cd docs/redesign/design-cinematic
PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs node tools/export-screens.mjs   # or: npm i -D playwright
```
