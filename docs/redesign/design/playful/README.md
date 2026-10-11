# momoshtrem design · Direction B, Playful cat-forward

The full redesign for Direction B in [`docs/redesign/design-prompt-playful.md`](../../design-prompt-playful.md): identity, design system, every screen at 390 / 768 / 1440 px in light and dark, and three clickable flows.

**Start here:** open [`index.html`](index.html) (it runs from the file system, or `npx serve docs/redesign/design/playful`). For build notes, read [`HANDOFF.md`](HANDOFF.md).

| Path | What |
|---|---|
| `index.html` | The hub: logo explorations and final set, Momo, voice, tokens, every component state, the screen viewer and the three prototypes. |
| `app.html` | The responsive prototype. `app.html#/for-you`; add `?theme=dark`, `&safe=1` (iPhone safe areas), `&flow=1` / `2` / `3` (guided flows), `&motion=reduce`. |
| `HANDOFF.md` | Layout grid, component inventory, per-screen behaviour, interaction and motion specs, accessibility, data additions for PLAN §6. |
| `tokens/` | `theme.css` (Tailwind v4 `@theme`), `tokens.css`, `tokens.json`, `CONTRAST.md`. Generated; edit `tools/tokens.source.mjs`. |
| `brand/logo/` | Mark, flat mark, mono mark, wordmark, lockups (light, dark, mono), app icon, maskable icon, favicons (SVG). |
| `brand/png/` | App icon 1024, apple-touch-icon 180, maskable 512/192, icons 512/192, favicons 16/32/48 and `favicon.ico`. |
| `brand/explorations/` | The three logo explorations. |
| `brand/mascot/` | Momo: sleeping, peeking, peeking-shy, knocking, proud, detective, yawn, avatar. |
| `exports/` | Static PNGs of every screen and state (see `exports/README.md`). |
| `assets/` | The prototype’s CSS, JS (no build step), self-hosted fonts and a Lucide icon subset. |
| `tools/` | Generators: tokens, icons, logo set, mascot, PNG renders and screen exports. |

Regenerate:

```bash
cd docs/redesign/design/playful
node tools/build-tokens.mjs                 # tokens/*
python3 tools/build-brand.py                # brand/logo, brand/explorations (needs fonttools)
python3 tools/build-mascot.py               # brand/mascot, assets/js/mascot.js
NODE_PATH=$(npm root -g) node tools/render-brand.cjs   # brand/png (Playwright)
NODE_PATH=$(npm root -g) node tools/export-screens.cjs # exports/*.png (Playwright)
```

Fonts: Fredoka, Atkinson Hyperlegible Next and Atkinson Hyperlegible Mono (SIL OFL, Google Fonts). Icons: Lucide (ISC). Artwork in the prototype is generated placeholder art.
