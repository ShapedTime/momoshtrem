# momoshtrem · Cinematic dark · handoff notes

Design exploration **A** from `docs/redesign/design-prompt-cinematic.md` (PLAN.md task D1). Everything here is runnable HTML/CSS/JS. Every screen is built from the same tokens and component classes the React app should use, so treat the prototype as a spec you can inspect in devtools.

| Path | What it is |
|---|---|
| `index.html` | Overview, with the three clickable flows in an inline player |
| `system.html` | Design system: logo, Noir, colour, type, spacing, elevation, motion, icons, every component with its states |
| `screens.html` | Every screen and state at 390 / 768 / 1440, rendered live |
| `prototype.html` | The whole prototype (hash routes, see §7) |
| `screens/<screen>/<state>-<width>.png` | 171 static exports of every screen and state (regenerate with `tools/export-screens.mjs` and `tools/optimize-pngs.py`) |
| `css/tokens.css` | **Tokens** as CSS custom properties: colour incl. status, type, spacing, radii, elevation, blur, motion, z-index |
| `tokens/theme.css` | Tailwind v4 CSS-first `@theme inline` mapping of those tokens |
| `css/ui.css` | Component styles (1:1 with the inventory in §3) |
| `css/app.css`, `css/screens/*.css` | Shell, hero/detail patterns, screen-specific layout |
| `assets/logo/*.svg` | Mark, mono, wordmark, lockups, app icons (1024, 180), maskable 512, favicons (16, 32), the 3 explorations |
| `js/*.js` | Prototype runtime (`core.js`), fixtures (`data.js`), placeholder artwork (`art.js`), icons, brand geometry |

The artwork is placeholder SVG, deliberately in every colour (Bluey sky-blue, Pluribus yellow, The Studio white) to prove legibility. In the app it is TMDB imagery.

---

## 1. Direction in one paragraph

Cinema lobby after dark. Near-black with a cool blue-violet bias (`--ink-1 #0B0D12`), surfaces that step up in small increments, translucent borders, and glass only where chrome sits over artwork. One accent, **cat-eye amber `#F2B33D`**, marks the next thing to do. Lavender was shown side by side (system.html#accent) and rejected because it collides with the needs-review violet. Type is **Geist** (UI and display, tight tracking on large sizes) plus **Geist Mono** for release names, paths and hashes. Motion is slow (220–900 ms), uses expo-out easing, and has a still fallback everywhere. The mascot is **Noir**, a black cat: in the dark you see only its amber eyes. The eyes are the loader, the favicon's cut-outs and the face of "Ask momoshtrem".

## 2. Layout grid

| Range | Name | Gutter | Columns / gap | Navigation | Overlays | Poster track |
|---|---|---|---|---|---|---|
| 320–639 | Phone | 16 | 4 / 12 | Bottom tab bar (56 + safe area), large titles, phone bar over heroes | Bottom sheets | `minmax(104px, 1fr)`; exactly 3 columns ≤ 380 px |
| 640–1023 | Tablet | 24 | 8 / 16 | Bottom tab bar | Sheets centred, max 640 | 140 |
| 1024–1439 | Laptop | 40 | 12 / 20 | Glass top bar (64), ⌘K, Ask pill, cat menu | Right-hand panel, 760 (480 narrow), inset 12 | 168 |
| ≥ 1440 | Desktop | 56 | 12 / 24 | Same | Same | 184; content max 1600 |

- **No horizontal page scroll at 320 px.** Every screen in the gallery is checked at 320, 390, 768 and 1440 (`scrollWidth === innerWidth`). The only horizontal scrollers are rails and the cast row; each scrolls inside its own container and bleeds to the screen edge through a negative margin equal to the gutter.
- **Poster grid:** `grid-template-columns: repeat(auto-fill, minmax(var(--poster-min), 1fr))`. Cards never have a fixed width, which fixes today's overlapping posters.
- **Rails:** column width is a fraction of the viewport (2.6 visible on phone, 4.4 on tablet, 7 on laptop, 8 at 1440), so the next card always peeks. Arrow buttons sit in the section head and are always visible on `(hover: hover) and (pointer: fine)`. They are disabled at either end and scroll 85% of the rail.
- **Detail pages:** hero grid of poster 240, info `1fr` and primary panel 360 (400 at 1440). Body grid is main `1fr` plus a 360/400 aside. On phone everything stacks, and the primary button moves into a thumb-reach **action dock** above the tab bar.
- **Safe areas:** the tab bar, action dock, sheet foot and phone bar all add `env(safe-area-inset-*)`. Use `viewport-fit=cover`, `theme-color #0B0D12`, `apple-mobile-web-app-status-bar-style black-translucent`.

## 3. Component inventory

Class names are in `css/ui.css`, render functions in `js/core.js` (`MS.ui.*`). Suggested React names follow PLAN.md F1.

| React component | Prototype | States / variants | Notes |
|---|---|---|---|
| `Button` | `.btn` + `-primary/-secondary/-ghost/-glass/-ready/-danger/-danger-quiet`, `-sm/-lg/-icon/-block` | default, hover, focus, active (scale .98), disabled, loading (spinner, label kept for width) | Never forced full width. `-sm` is 36 visual / 44 hit. Solid danger appears only inside confirm dialogs. |
| `IconButton` | `.btn.btn-icon` | same | Always has `aria-label`. |
| `Input`, `SearchInput`, `Select`, `Textarea` | `.input`, `.input-wrap`, `select.input` | default, hover, focus (amber ring), error (`aria-invalid`), disabled | 44 tall, **16 px text** (no iOS zoom). |
| `Switch`, `Range` | `.switch`, `.range` | on/off, focus | Dual-thumb range for year, size. |
| `Chip` (attribute) | `.chip` + `--hi/--accent/--pack/--warn/--err/--glass` | static | Parsed release attributes. |
| `FilterChip` | `.fchip` | off, on (`aria-pressed`), zero (dimmed, still visible), removable (`.x`), menu (`--menu`) | Always shows a facet count. |
| `SegmentedControl` | `.seg` (`--block`) | selected | Mode of the same content (scope, density, grid/list). |
| `Tabs` | `.tabs` | selected, with counts | Sibling content (seasons incl. **Specials last**, Movies/Shows). |
| `StatusPill` | `.pill--ready/downloading/missing/review/watched/upcoming/warning/error/neutral/accent`, `--on-art`, `--lg` | — | Always icon + word. Missing is dashed. Downloading embeds a ring with %. Vocabulary in `STATUS` (core.js). |
| `Confidence` | `.conf--high/medium/low/none` | — | Three bars plus a word. |
| `Progress`, `Ring`, `Steps` | `.progress(--lg/--accent/--ready/--watched/--indeterminate)`, `.ring`, `.steps(--inline)` | done, active, pending | Steps are used for mapping jobs, Ask and Trakt sync. |
| `Poster`, `Backdrop`, `Still` | `.media--poster/--wide/--square` | placeholder = surface-2 | Inner 1 px highlight ring. |
| `PosterCard` | `.pcard` | default, hover/focus lift, with status pill, with reason + feedback, with watched bar, dismissed | **Title and year are always visible**, 2-line clamp. |
| `EpisodeCard` (landscape) | `.lcard` | per availability: ready, downloading %, needs torrent, not in library, airs on | The action matches availability (Play in Infuse / Details / Find / Add / none). |
| `EpisodeRow` | `.eprow` (`--rich`, `.is-next`, `.is-watched`) | watched, ready, downloading, missing, upcoming, needs review | Compact 60 px row by default. The next-up row gets an amber rule and an "Up next" label. |
| `ReleaseCard` | `.release` (`.is-best/.is-dim/.is-added`) | best match, normal, warning, no seeders, added | See §5.2. |
| `TorrentCard` | `.tcard` (`.is-streaming`) | streaming, downloading, paused, idle/complete, weak health | Links to its title. |
| `Panel`, `ListRow`, `Banner` | `.panel`, `.list-row`, `.banner--review/warning/error/ready/accent` | — | Banners carry one action at most. |
| `Sheet` | `.sheet` (`--full/--narrow/--nested`), `.sheet-grabber/-head/-body/-foot` | open, nested | §4.1 |
| `Dialog` | `.dialog` | — | Confirmations only. |
| `Menu` | `.menu` | item, danger, hint | The phone version is a sheet. |
| `Toast` | `.toast--ready/info/error` | auto-dismiss 5 s; sticky for errors and "ready" | §4.4 |
| `Skeleton` | `.skel`, `.skel-poster/-wide/-line` | shimmer; static under reduced motion | Same box sizes as the final content. |
| `EmptyState` | `.empty` + `ui.cat(sit|sleep|blank|peek)` | — | |
| `CatEyesLoader` | `brand.eyes()` / `.cat-eyes--svg` | blinking; static under reduced motion | |
| `CatAvatar` | `brand.avatar()` | — | The face of "Ask momoshtrem". |
| `CommandPalette` | `.palette` | query, grouped results, selected, empty | Recommend `cmdk`. |
| `TopBar`, `TabBar`, `LargeTitle`, `PhoneBar`, `ActionDock` | `.topbar(.is-over-art/.is-solid)`, `.tabbar`, `.large-title`, `.phone-bar`, `.action-dock` | current route, badge | |
| `Rail` | `.rail--posters/--wide`, `.rail-arrows` | arrows disabled at the ends | |
| `PrimaryPanel` | `.primary-panel` | none → needs_torrent → downloading → ready | §5.3 |

## 4. Interaction details

### 4.1 Sheets
- **One component, three presentations:** a bottom sheet below 640, a centred bottom sheet (max 640) up to 1023, and a right-hand panel from 1024. Use Radix Dialog (focus trap, `aria-modal`, Esc, scroll lock) with Vaul for drag-to-dismiss on touch.
- **Height:** content sheets size to their content up to `100% − 12px − safe-top`. **Search sheets (`--full`) open at full height immediately**, so loading results never changes the height and taps never land on the backdrop.
- **Structure:** the grabber (20 px hit area) and head (title, close, search, chips) are sticky at the top. The body scrolls with `overscroll-behavior: contain`. The foot (primary action) is sticky at the bottom on glass and includes the safe area.
- **Nested sheets** (Filters, Sort, Inspect, Episode picker) stack on top: z 55, max height 86% on phone, and a 420 px inset panel on desktop with a dimmed parent. Closing returns focus to the opener. Esc closes only the top sheet.
- **Close never loses work:** torrent search keeps query, scope, filters and sort per context for the session (`app.state.ts[contextKey]`), and shows "Picked up where you left off" on reopen.
- **URL:** sheets open from route params (`?sheet=torrent&show=frieren&se=S01E29`), so Back closes them and states are deep-linkable.
- **Motion:** 360 ms `--ease-sheet` up (desktop: 32 px slide plus fade). The scrim fades in over the same duration with a 4 px blur.

### 4.2 Sticky elements
- **Top bar:** transparent with a top scrim over heroes; becomes `--glass-bg-strong` plus a hairline once `scrollY > 120`.
- **Tab bar:** always fixed, glass. Activity shows a count of active torrents.
- **Phone detail pages:** the action dock (primary action and one icon action) sits above the tab bar with a gradient fade behind it. The in-page primary panel still shows the state, steps and hint.
- **Mapping review:** the sticky foot shows "Apply N mappings" with a live count and stays disabled until nothing needs review (or you choose to apply anyway).
- **Settings:** a save bar appears only when something changed.
- **Episode list:** the season tabs and range chips scroll with the page (they're not sticky), because the page is already short (§5.4).

### 4.3 Loading choreography
1. **Nothing for the first 150 ms** (most cached loads finish). Then skeletons the exact size of the final layout. Never a full-page spinner.
2. **Swap skeleton → content** with a 220 ms crossfade, no movement. Images fade in on load over a `--surface-2` placeholder.
3. **Hero:** the backdrop crossfades over 900 ms. The ambient glow (`--ambient`) eases to the new colour. Picks don't rotate on every load: the order is stable per day, and the pager changes them on request.
4. **Page navigation:** View Transitions crossfade (560 ms). Poster → detail uses a shared element (`view-transition-name: hero-poster` on the clicked poster and the detail poster). In Next 16, use React's `<ViewTransition>` (with `experimental.viewTransition`), or `document.startViewTransition` around `router.push`.
5. **Long jobs** (mapping, Ask, Trakt sync, recommendation refresh) show named steps, never just a spinner. Each step shows a detail ("12 of 16"). Noir's eyes appear only while the user is actually waiting.
6. **Polling** (downloads, jobs) pauses when the tab is hidden (TanStack Query `refetchIntervalInBackground: false`). Values animate (bar width 360 ms); numbers update in place with tabular figures.

### 4.4 Feedback
- **Toasts:** one at a time, newest on top. Success and info dismiss after 5 s; errors and "ready" moments stay until dismissed. At most one action (Undo, Retry, How to play).
- **Destructive actions** are reached through an overflow menu (`…`), styled as quiet red text, and confirmed in a dialog that states the consequence ("Downloaded data for 2 torrents is deleted. Your Trakt history isn't touched."). Removal from a list offers Undo for 10 s.
- **Recommendation feedback:** thumbs up/down inline; "Not interested" and "Already seen" in a sheet. The card dims immediately, with an Undo toast.

### 4.5 Keyboard and focus
- **Shortcuts:** ⌘K / Ctrl K / `/` open the palette. Esc closes the top overlay. In the palette, ↑ ↓ move, ↵ opens, ⌘↵ runs the secondary action (e.g. "Find a torrent").
- **Focus ring:** 2 px canvas gap plus a 2 px amber ring (`--focus-ring`). Posters show the ring on the artwork, offset 3 px, and lift.
- **Skip link** to main content. Every icon-only button has an `aria-label`. Status is never colour alone.

### 4.6 Reduced motion
Under `prefers-reduced-motion: reduce` (ui.css bottom):
- Transitions become ≤ 1 ms, and view transitions are disabled.
- Parallax and hover lifts are off, shimmer becomes a static block, and the eyes don't blink.
- Spinners become static rings, and indeterminate bars a static 40% fill.
- Sheets and dialogs appear with a 120 ms fade.

## 5. Screen rules worth calling out

### 5.1 Long text
| Text | Rule |
|---|---|
| Release names | Geist Mono 13/1.45. **Break only after `.`, `-`, `_`, `]`, `)`, `/`** (insert `<wbr>`, dim the dots), then `overflow-wrap:anywhere` as a last resort. Never truncate in search results, because the name is the decision. In compact contexts (activity cards) clamp to 2 lines with the full name in `title` and in the detail view. |
| File paths | Same break rule plus spaces. The folder part is dimmed and the file name stays full. |
| Titles on cards | 2 lines, then ellipsis. Always visible. |
| Titles in heroes | `text-wrap: balance`, never clamped. |
| Overviews | 3 lines (4 on desktop), with "More" on tap. |
| Episode names in rows | 1 line with ellipsis; the full name is on the episode sheet. |

### 5.2 Torrent search
- **Context pre-fill:** a movie gives `Title Year`. An episode gives `Show SxxEyy`. Anime uses the short title with an absolute number (`Frieren 29`) and shows the extra queries it also ran ("Sousou no Frieren 29 · Frieren S02E01 (scene numbering)").
- **Scope switch:** Episode · Season pack · Any. When several episodes in the season are missing, a banner suggests packs.
- **Best match:** one card with an amber outline, the score, and up to 4 reasons drawn from `score_reasons` ("2160p WEB-DL · Dolby Vision · 4.1 GB · 312 seeders · matches your quality profile"). Its Add is the only primary button in the list. Every other Add is secondary.
- **Card layout:** release name, attribute chips, then facts (size, seeders in status colour, leechers, age, indexer), then warnings (CAM, no seeders, over the size limit). Actions: **Add**, **Inspect** and copy magnet. On phone the actions sit in one row at natural width with the magnet icon pushed right, so they're hard to hit by accident. On desktop they form a fixed 132 px column on the right.
- **Filters:** quick chips in the head (resolution and, on wider screens, codec/DV), each with a facet count, plus a **Filters** chip opening a nested sheet with every facet and a **Sort** chip.
- **Zero results caused by filters** names the filters ("2160p and Dolby Vision together match nothing"), shows them as removable chips, and offers "Clear all".
- **No results** suggests broadening: a shorter query, a wider scope, the original title.
- **Partial failure:** a warning banner names the indexer and offers Retry.
- **Searching:** a per-indexer progress list with skeleton cards in final dimensions.
- **Paging:** "Showing 6 of 45" plus **Load more** (append, keep scroll position).
- **Inspect** is a dry-run job (`POST /search/torrents/inspect`): file list grouped as will-map / you'll-review / specials / ignored. Its primary action is "Add & review mapping" (shows) or "Add to library & download" (movies).

### 5.3 Primary action state machine (detail pages)
| Stage | Panel | Primary button | Secondary |
|---|---|---|---|
| `none` | "Not in your library" + episode count; adding is free | **Add to library** | — |
| `needs_torrent` | What's missing ("Episodes 29–38 have no torrent") | **Find a torrent** / **Find episodes 29–38** | — |
| `downloading` | % and speed, "You can start watching in Infuse now" | View download | How to play in Infuse |
| `ready` | Green "Ready in Infuse", what's ready, **WebDAV path with Copy** | **How to play in Infuse** (sheet) | Copy path (dock icon) |

A four-segment step bar (done green, current amber) sits above, labelled "Add · Torrent · Download · Ready". When a title is ready, the **Current torrent** card and **Subtitles** move to the top of the body, above the fold on desktop. "Search torrents" becomes "Replace" inside that card.

### 5.4 Long seasons
- Compact rows (60 px) by default, with a density toggle to show stills.
- **Range chips:**
  - Ranges of ~10 for anime cours (1–10 · 11–20 · 21–28 · 29–38).
  - TMDB episode-group arcs for very long shows (Egghead · 1086–1155, Wano · 892–1085).
  - Each chip shows a "missing" count where relevant.
- **Jump to episode** takes an absolute or in-season number, scrolls to the row and flashes it.
- Fully-watched earlier episodes collapse into one "Show episodes 1–20 (all watched)" row.
- Use `@tanstack/react-virtual` for > 60 rows.
- A 38-episode season (Frieren, 390 px wide) now takes about 1,700 px instead of about 8,800.

### 5.5 Mapping review
- **Groups:** Matched · Needs review · Specials · Extras & ignored · Unmatched.
- **Row:** file (wrapping path) → target episode with confidence and a reason. AI reasons are prefixed with a sparkle and "AI:".
- **Episode picker:** a searchable list, grouped by season, with Specials included and already-assigned episodes marked. Phone: a nested sheet. Desktop: the right-hand pane.
- **Re-check with AI** takes an optional hint ("these are absolute numbers").
- **Apply:** the sticky "Apply N mappings" button (see §4.2).

## 6. Data the screens need that PLAN.md §6 doesn't list yet

| Screen | Field / endpoint | Why |
|---|---|---|
| All heroes | `backdrop_dominant_color` (hex) on titles, rec items and up-next | Ambient hero glow. Compute server-side when caching images (e.g. a 32×18 resize and average), store in `tmdb_cache`. |
| For You | `GET /recommendations/home` → `hero_picks: [{tmdb_id, media_type, kicker, reason}]` (2–3) | "Tonight's pick" hero with a reason, stable per day |
| For You | `newEpisodes`: `GET /me/new-episodes?days=7` → `[{show, episode, aired_at, availability}]` | "New episodes" rail |
| Up next / episodes | `episode.name`, `episode.still_path`, `absolute_number` in `/me/up-next` | Landscape cards show the still, name and `#1124` |
| Detail | `watch_state: {watched_at, plays, rating, progress: {completed, aired}}` and `recommendation_reason` (already noted) plus `webdav_path` | The Trakt line, "Why it's for you", and the copyable path |
| Detail | `availability_stage: none \| needs_torrent \| downloading \| ready` plus `download: {pct, speed, eta, seeders}` | Drives the primary state machine without client-side inference |
| Show detail | Per-season `{episodes_total, ready, missing, needs_review}` and per-episode `state` (`watched/ready/downloading/missing/upcoming/needs_review`) with `pct` and `air_date` | Season tab counts, row pills |
| Show detail | Ranges: `GET /shows/{id}/ranges` (from the episode group or cours of 10–13) | Range chips and arcs |
| Torrent search | `queries` exists; add `indexers: [{name, status: ok\|timeout\|error, count, ms}]` and stream progress (SSE, or `partial: true` plus polling) | Per-indexer progress and partial-failure banner |
| Torrent search | `facets` counts **under the current filters** (and a `would_be` count for each disabled option) | Filter chip counts; "Show N results" |
| Torrent search | `age` or `published_at` (exists) and `release.why_short` (≤ 4 short phrases, e.g. from `score_reasons`) | The best-match "why" line |
| Library | `size_bytes`, `last_watched_at`, `added_at` and `needs_review` count per item | Sort and list view |
| Mapping review | Now in PLAN §6 "Assignment": the job's `item`, `torrent` facts, `progress` step details, `suggested_hints`, `ai_available` and live `counts`; per proposal its `group`, `targets`, `replaces`, `updated`/`hinted` and `extra_type`; `GET /shows/{id}/episodes` for the picker | Header facts, named steps, summary line, rows, Re-check chips, picker tags |
| Activity | Now in PLAN §6 "Torrents and activity (B6)": `GET /torrents/summary` → speeds, a 60 s `history`, `counts` per state and `disk`; `GET /torrents` items with `state`, `loaded`, `titles` (with `scope`) and `open_job` | Summary strip, the four sections |
| Activity | Per torrent: `readers: [{client: "Infuse", device, file, episode, offset, rate_bps}]` (from WebDAV request User-Agent and range reads), `pieces: bitfield`, `priority_pieces` | "Streaming now", active readers, piece map (#99: streaming sessions and the detail) |
| Ask | `POST /recommendations/ask` events: add `refinements: [string]` in `done` and `match_tag` per `item` | Refine chips and the one-line match tag on cards |
| Settings | `GET /status` per-indexer health; `GET /settings/ai/usage` → `budget_usd`, `by_feature` | Status list, budget bar |
| Settings | Quality profile preview: `GET /settings/quality-profile/preview` → a one-line summary | "Best match would prefer…" |
| Subtitles | `match: exact_release \| same_source \| different_timing` and `downloads` per result | Ranked results with match pills |

**Activity and on-demand torrents (B6).** Torrents don't download in the background: pieces are fetched around what's being played and kept in a cache (4 GB by default) that drops the oldest, so a torrent's progress can go down, and one the library knows may not be loaded at all. So:
- **No ETA**, anywhere: there's no finish time to promise.
- **The fourth section is Idle**, not Seeding. It holds torrents that are loaded but quiet, and torrents that aren't loaded (`loaded: false`). Show those without a progress bar, peers or speeds (they're null), not as "0%".
- **The pill says "Complete" only when `complete` is true.** A loaded torrent that's idle and incomplete is just "Idle".
- **Pause is sticky.** A paused torrent stays paused across restarts, and playing it in Infuse doesn't wake it; only Resume does. Pause all covers the streaming and downloading torrents.

## 7. Prototype routes (for reference while building)

These hash routes are what `screens.html` and `js/manifest.js` enumerate:

| Screen | Routes |
|---|---|
| Sign in | `/signin` |
| For You | `/home` |
| Ask | `/ask`, `/ask/:thread` |
| Browse and search | `/discover`, `/search` |
| Detail | `/title/movie/:id`, `/title/tv/:id?season=n&view=rich` |
| Mapping review | `/mapping` |
| Library | `/library` |
| Activity | `/activity`, `/activity/:hash` |
| Settings | `/settings?section=trakt\|ai\|quality\|playback\|status` |
| System states | `/states`, `/offline`, `/not-found` |

Add these params to any route:

| Param | Effect |
|---|---|
| `state=…` | Pick a screen state |
| `sheet=torrent&show=…&se=…` / `movie=…` | Open the torrent search sheet |
| `sheet=…` (other sheets) | Open that sheet |
| `palette=1&q=…` | Open the command palette |
| `dialog=remove` | Open the remove confirmation |
| `toast=ready` | Show the "ready" toast |
| `reset=1` | Reset flow state |

## 8. Build notes
- **Fonts:** `next/font/google`, `Geist({ subsets:['latin'], variable:'--font-geist-sans' })` and `Geist_Mono` (`--font-geist-mono`). The poster faces (Bebas Neue, Cormorant) are prototype-only.
- **Icons:** `lucide-react`. Names match `js/icons.js` keys, camel-cased (`scanSearch` → `ScanSearch`, `sliders` → `SlidersHorizontal`, `library` → `LibraryBig`, `arrowUpDown` → `ArrowUpDown`, `captions` → `Captions`, `sparkle` → `Sparkle`).
- **Brand assets:**
  - `app/icon.svg` ← `favicon-32.svg`, and `app/apple-icon.png` ← render `app-icon-180.svg`.
  - Manifest icons ← `app-icon-1024.svg` (192/512 PNG) and `maskable-512.svg` (`purpose: "maskable"`).
  - `theme_color` and `background_color` are `#0B0D12`.
- **Libraries:**
  - Sheets: Radix Dialog + Vaul.
  - Palette: `cmdk`.
  - Long lists: `@tanstack/react-virtual`.
  - Data: TanStack Query (PLAN F1).
- **Status vocabulary:** keep wording and icons in one module (see `STATUS` in core.js) so every surface says the same thing. Today the grid says "In Library" while detail says "Ready to Stream".
- **Contrast** (system.html computes these live):

  | Token | Ratio |
  |---|---|
  | Token | On canvas | On `--surface-2` |
  |---|---|---|
  | `--text-1` | 17.1:1 | 15.1:1 |
  | `--text-2` | 9.0:1 | 7.9:1 |
  | `--text-3` | 5.3:1 | 4.7:1 |
  | Status colours | 7.2–13.5:1 | 6.4–12:1 |
  | `--accent-ink` on `--accent` | 9.9:1 | — |

  `--text-4` is decorative only.

## 9. Open questions for the pick (PLAN D2)
1. **Mascot name.** "Noir" is proposed. Keep "Momo" if it means something to you.
2. **Top bar vs side rail on desktop.** The top bar was chosen because heroes are full-bleed and the nav has only five items. A slim rail would suit a denser control-room feel if Activity becomes the main use.
3. **Merging with exploration B.** If you merge with the playful exploration, the cat moments are isolated in `ui.cat()`, `brand.eyes()` and `brand.avatar()`, so they can be swapped without touching layouts.
