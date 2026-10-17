# momoshtrem web: style guide

How the UI looks and behaves. The **Cinematic dark** design in
`docs/redesign/design-cinematic/` is the spec. `HANDOFF.md` explains it,
`screens/` shows every screen and state, and `prototype.html` is clickable.
This guide covers how that design is built in `web/`. The live component
gallery is **`/dev/ui`** (dev server only).

## 1. Principles

- **Cinema lobby after dark.** Near-black surfaces with a cool bias. Glass only where chrome sits over artwork.
- **One accent.** Cat-eye amber (`--accent`) marks *the next thing to do*. Use one primary button per view.
- **Status is never colour alone.** Every state is an icon plus a word from the shared vocabulary (§5).
- **Nothing hover-only.** Titles, actions and status are visible on touch.
- **Calm, specific copy.** Say what happened, what's fine and what to do next. Noir (the cat) shows up for empty states and long waits, never when something important broke.
- **Phone and laptop are equals.** Check every screen at 390 and 1440 px and make sure nothing scrolls sideways at 320 px.

## 2. Tokens and Tailwind

Raw tokens live in `src/styles/tokens.css`, ported from the design. The
Tailwind v4 `@theme` in `src/app/globals.css` maps them to utilities. **Use
tokens, never raw hex values or Tailwind's default palette.** The default
palette only remains for legacy pages.

| Need | Utilities / tokens |
|---|---|
| Surfaces | `bg-bg` (canvas), `bg-surface-1` (cards, rows), `bg-surface-2` (raised, inputs), `bg-surface-3` (hover), `bg-surface-4` (pressed, tracks), `bg-ink-0` (deepest) |
| Lines | `border-line-1` (hairlines), `border-line-2` (cards, inputs), `border-line-3` (hover, strong) |
| Text | `text-fg-1` primary, `text-fg-2` secondary, `text-fg-3` meta (AA on every surface), `text-fg-4` decorative only |
| Accent | `bg-accent` / `text-accent-ink` (fills), `text-accent-text` (amber text), `bg-accent-soft`, `border-accent-line` |
| Status | `text-ready`, `bg-ready-bg`, `border-ready-line`; likewise for `downloading`, `missing`, `review`, `watched`, `upcoming`, `warning` and `error` |
| Type | `text-display`, `text-title-1/2/3`, `text-body-l`, `text-body`, `text-small`, `text-micro`, `text-mono`, or the `.t-*` classes (with clamp and balance) |
| Radius | `rounded-xs` (6), `-sm` (8, inputs), `-md` (12, buttons, posters), `-lg` (16, panels), `-xl` (22, dialogs), `-2xl` (28, sheets) |
| Elevation | `shadow-e1` … `shadow-e4`, `shadow-lift` (hover lift), `shadow-accent-glow` |
| Motion | `ease-out` (enter), `ease-in` (exit), `ease-in-out` (crossfade), `ease-sheet`, `ease-spring`; durations via `duration-(--d-fast/base/slow/slower)` |
| Layers | `z-(--z-topbar)`, `z-(--z-tabbar)`, `z-(--z-sheet)`, `z-(--z-dialog)`, `z-(--z-toast)` … |

**Component classes.** `src/styles/components.css` holds the prototype's
component classes, such as `.btn`, `.pill`, `.panel` and `.sheet`. They sit
in `@layer components`, so a utility class always overrides them. Prefer the
React primitive (§4) to the raw class.

**Fonts.** Geist (UI) and Geist Mono (release names, paths, hashes) come from `next/font`.
Use `.release-name` and `.path` for strings that must wrap only at separators.

## 3. Layout

| Range | Gutter | Navigation | Overlays | Poster track |
|---|---|---|---|---|
| 320–639 | 16 | Phone bar + bottom tab bar (56 + safe area) | Bottom sheet | 104 (exactly 3 columns ≤ 380) |
| 640–1023 | 24 | Phone bar + tab bar | Centred bottom sheet (≤ 640) | 140 |
| 1024–1439 | 40 | Glass top bar (64) | Right-hand panel (760; narrow 480) | 168 |
| ≥ 1440 | 56 | Top bar | Right-hand panel | 184; content max 1600 |

- **Page frame:** wrap content in `.page`, which adds gutters and a max width. The shell provides `<main id="main">`, so pages never render their own `<main>`.
- **Grids** use fluid columns: `<PosterGrid>`, i.e. `repeat(auto-fill, minmax(var(--poster-min), 1fr))`. **Never give cards a fixed width.**
- **Rails** (`<Rail>`) scroll inside their own container and bleed to the screen edge. The next card always peeks. Arrows appear only with a fine pointer.
- **Breakpoints:** Tailwind `sm` = 640, `lg` = 1024, `xl` = 1440. `md` (768) is legacy and not part of the design.
- **Safe areas:** anything fixed to an edge (tab bar, sheet foot, action dock) adds `env(safe-area-inset-*)`.
- **Task pages** (the mapping review, `/mapping/{id}`; Ask momoshtrem, `/ask`) have their own back link and sticky foot. Their route wraps the page in `<div className="task-page">`. `shell.css` then hides the phone bar and tab bar (the desktop top bar stays), drops the main area's bottom padding and, below 1024 px, moves toasts to the bottom edge. The shell still renders both bars and CSS hides them, so the server's HTML is right from the first paint, with no flash or shift.

## 4. Components

All primitives live in `src/components/ui` (barrel: `@/components/ui`). The
logo and Noir live in `src/components/brand`. See every state at `/dev/ui`.

| Need | Use | Rules |
|---|---|---|
| Actions | `Button`, `IconButton`, `ButtonLink` | One `primary` per view. Solid `danger` only inside confirm dialogs; triggers are `danger-quiet` or icon buttons. `size="sm"` keeps a 44 px hit area. `loading` keeps the label width. Never forced full width (use `block` when you mean it). |
| Text entry | `Input`, `SearchInput`, `Select`, `Textarea` | Text is 16 px or larger (no iOS zoom). Pass `label`, or `aria-label` without one. `error` sets `aria-invalid` and the message. |
| Toggles | `Switch`, `FilterChip`, `SegmentedControl`, `Tabs` | A FilterChip always shows its facet count, dimmed at zero. SegmentedControl switches modes of the *same* content; Tabs switch *sibling* content (Specials last). |
| Tags | `Chip` | Parsed attributes (2160p, DV, Season pack). Not interactive. |
| Status | `StatusPill`, `Confidence`, `Progress`, `RingMeter`, `Steps` | Labels come from `lib/status.ts`. Long jobs show named `Steps`, never just a spinner. Indeterminate progress is for short waits only. |
| Media | `Media`, `PosterCard`, `PosterGrid`, `Rail` | The title is always visible (2-line clamp). The placeholder is `surface-2`; images fade in. |
| Containers | `Panel`, `ListRow`, `Banner` | A banner carries at most one action (an extra "Details" link is fine). |
| Loading | `Skeleton`, `PosterSkeletons`, `WideSkeletons`, `LoadingRegion`, `Spinner` | Skeletons match the final layout's size. `LoadingRegion` waits 150 ms before showing them. |
| Empty | `EmptyState` (`cat="sit|sleep|peek|blank"`) | One plain sentence about why it's empty, and the next step. |
| Waiting | `Loader` (Noir's eyes) | Only while the user is actually waiting on a long job. |
| Overlays | `Sheet`, `ConfirmDialog` / `useConfirm()`, `Menu`, `useToast()` | See §6. |
| Copyable values | `CopyField` | Works on plain HTTP too (falls back from the Clipboard API). |

## 5. Status vocabulary

`src/lib/status.ts` is the **only** place status wording and icons are
defined. Every surface (cards, rows, panels, Activity, Settings) uses
`<StatusPill status="…">`.

`ready` · `ready_infuse` · `downloading` · `partial` · `needs_torrent` · `missing` ·
`not_in_library` · `airs_on` · `upcoming` · `needs_review` · `watched` · `streaming` ·
`paused` · `idle` · `ok` · `unreachable` · `not_configured` · `warning` · `error`;
torrents: `healthy` · `no_seeders` · `added` · `complete` · `few_peers`; trackers: `working` · `status_unknown`; subtitle matches: `match_exact` ·
`match_same_source` · `match_different_timing` · `match_unknown`; files in a mapping
job: `matched` · `special` · `unmatched` · `ignored` (the mapping review's group titles,
summary words and foot messages live with its rules in `lib/mapping/review.ts`); downloads (#77):
`kept` · `smart` · `queued` · `waiting_for_stream` · `waiting_for_window` · `stalled` · `disk_full` ·
`storage_full` · `on_disk` · `no_file` · `not_keepable`, with `downloading` and `paused`
(`DOWNLOAD_STATE_STATUS` maps a download's state to its key; the copy that needs a number or a time,
"Downloading 42%", "Waits until 01:00", "Kept until watched · by Nov 16", lives in `lib/title/keep.ts`)

On narrow cards, pass a short `label` the design uses, for example "No torrent" or
"Review"; the tone and icon stay the same. If a new state is needed, add it
to the map and don't invent wording in a component.

## 6. Interaction

**Sheets** (`<Sheet>`):
- Presentation follows the width (§3); see HANDOFF §4.1.
- `size="full"` opens at full height immediately, so loading never moves it. Use it for search.
- `nested` stacks a sheet on top, and Esc closes only the top one.
- Opening a sheet moves focus into it, and closing returns focus to whatever opened it.
- **URL-driven sheets:** register in `src/lib/sheets/registry.tsx` and open with `useSheet().open('name', params)`. Back closes them and they can be deep-linked (`?sheet=…`).

**Feedback:**
- **Toasts** show one at a time. Success and info leave after 5 s; errors and `sticky` "ready" moments stay. At most one action (Undo, Retry, How to play).
- **Destructive actions** go behind a menu or quiet trigger, and are confirmed with `useConfirm()`. The confirmation says exactly what goes and what stays.
- **Removal from a list** offers Undo for 10 s.

**Keyboard and focus:**
- ⌘K / Ctrl K / `/` open the command palette, and Esc closes the top overlay. In the palette, ↑ ↓ move (and wrap), ↵ opens, and ⌘↵ (Ctrl ↵) runs the selected row's secondary action: torrent search (Find missing for a library title that lacks a file, Find a torrent for a TMDB title). The footer names it when the row has one.
- The focus ring is a 2 px canvas gap plus a 2 px amber ring.
- Every icon-only button has an `aria-label`, and the skip link goes to `#main`.

**Loading:**
- Show nothing for 150 ms, then skeletons, then a 220 ms crossfade to content. Never a full-page spinner.
- Polling uses TanStack `refetchInterval` (it pauses in hidden tabs), never `setInterval`.

**Motion:**
- Slow and eased (220–900 ms).
- Under `prefers-reduced-motion`, transitions become 1 ms, view transitions and hover lifts are off, shimmer and blinking stop, and spinners become static rings.

## 7. States every screen needs

| State | Pattern |
|---|---|
| Loading | Skeletons the size of the content (`LoadingRegion`) |
| Empty | `EmptyState` with the right cat and one next step |
| Filtered to nothing | `EmptyState cat="peek"` naming the filters, with removable chips and "Clear all" |
| Section failed | Inline `Banner tone="error"` with "Try again"; the rest of the page keeps working |
| Backend down | Handled by the shell: `ConnectionBanner` → `/offline` (retries on its own, says honestly whether Infuse still plays) |
| Device offline | Handled by the shell: "You're offline" banner |
| Not found | `notFound()` → "Nothing here but the cat" inside the shell |
| Crash | `error.tsx` → `ErrorView` (what failed, what's fine, Try again) |

## 8. Detail pages

The movie and show pages (`/movie/[id]`, `/tv/[id]`) share `components/title/`
and the pure helpers in `lib/title/`. Build any page about one title from them.

- **One state machine.** The server sends `library.availability_stage`; `primaryActionFor(title)`
  (`lib/title/stage-copy.ts`) turns it into the panel's words and buttons:
  Add to library → Find a torrent ("Find episodes 29–38") → Downloading → Ready in Infuse.
  Never infer the stage on the client and never promise a finish time: downloading says it
  streams now. `PrimaryPanel` shows the four steps, the hint and the copyable WebDAV path.
- **Action dock.** Below 1024 px the panel's button moves to `ActionDock`, a thumb-reach bar
  above the tab bar (plus Copy path once ready). Hero pages call `useOverArt()`: the main area
  slides under the top bar and the page draws its own phone bar (Back, ⋯).
- **Once ready,** the current torrent and the subtitles move to the top of the body.
  Destructive actions (Remove from library, Remove file) sit in the ⋯ menu or a quiet button and
  are confirmed with what goes and what stays.
- **Episode rows** (`EpisodeRow`) are compact (60 px) or, with `?view=rich`, have a still
  (92 px). Each state gets its pill from `lib/status.ts` and one action: find (missing), ⋯ for the
  episode sheet (has a file, or watched without one), a chevron to the mapping review (needs
  review), nothing until it airs. "Up next" with the amber rule is Trakt's next episode
  (`watch_state.next_episode`), else the first frontier episode (`nextUpEpisode`). Watched wins
  the pill, and a watched episode's file keeps its actions. With absolute ordering a row
  reads `#1100 · S22E15`, unless the absolute number is the in-season one.
- **Long seasons** (HANDOFF §5.4):
  - range chips from `SeasonDetail.ranges`, the frontier's pressed, each jumping to its start;
  - Jump to episode, which reads `29`, `1100` or `S02E05` (`parseJump`) and switches `?season=` when needed;
  - leading watched runs (10 or more) collapse into "Show episodes 1–20 (all watched)", never past the Up next row;
  - seasons over 60 rows render through the window virtualiser (`@tanstack/react-virtual`).
  Rows outside the window aren't in the DOM, so jump with `scrollToEpisode`, never by element id.
- **Download & keep** (#206): the Keep sheet (`KeepSheet`) opens from a ready movie's second
  action, the ⋯ menu ("Download & keep show"), the season row and the episode sheet. Once kept, the
  same places offer Change duration (the sheet) and Stop keeping (confirmed). `KeepStatus` shows what
  holds a title on its panel, and `EpisodeRow` gets one keep pill beside its state ("Kept", "Smart", or
  why the file isn't really held). The words come from `lib/title/keep.ts`.
- **URL state.** `?season=`, `?view=rich`, the sheets (`?sheet=episode&tmdb=&se=S01E05`,
  `?sheet=show-settings&tmdb=`, `?sheet=infuse-setup&path=&url=`, `?sheet=keep&movie=` or
  `?sheet=keep&show=&scope=show|season|episode` with `season` or `se`), and the one-off `?ep=` that a
  cross-season jump scrolls to.

## 9. Copy

- Sentence case everywhere. Contractions are fine ("isn't", "you're").
- Name the thing: "Remove Severance from your library?", not "Are you sure?".
- Errors explain the cause and the way forward: "Check that caps lock is off." Never blame the user.
- Talk about Infuse and Apple TV by name; that's where people watch.
- Don't promise what the system can't know. For example, the offline page only claims Infuse still plays when WebDAV actually answered.

## 10. Building a screen

1. **Contract:** types for its endpoints in `src/lib/api/contract/<domain>.ts`, matching PLAN.md §6. Change PLAN.md in the same PR if the contract changes.
2. **Data:** calls and `queryOptions` in `src/lib/api/endpoints/<domain>.ts`, and fixtures and handlers in `src/mocks/` so the screen works in mock mode (`NEXT_PUBLIC_API_MOCKING=enabled npm run dev`) before the backend lands.
3. **UI:** compose primitives. Screen-specific CSS can live next to the route (see `app/(auth)/login/auth.css`), in `@layer components`, ported from `docs/redesign/design-cinematic/css/screens/`.
4. **Navigation:** update its entry in `src/lib/nav.ts` if it's a top-level destination.
5. **Tests:** a Playwright spec for the main flow on both projects (390 phone, 1440 desktop), with `expectNoA11yViolations` and `expectNoHorizontalOverflow` (also at 320).
6. **Screenshots:** add cases to `e2e/screenshots.spec.ts`, then run `SCREENSHOTS=1 npx playwright test screenshots && node scripts/optimize-screenshots.mjs` and link them in the PR.

## 11. Checklist (PLAN.md §7 "Acceptance for every F task")

- [ ] Matches the design at 390 and 1440 px; no horizontal overflow at 320 px.
- [ ] Titles and actions visible without hover; touch targets ≥ 44 px.
- [ ] Destructive actions quiet and confirmed.
- [ ] Fluid grids (no fixed card widths in fixed tracks).
- [ ] Loading, empty and error states in place.
- [ ] Keyboard and screen-reader basics: labels, focus order, Esc, visible focus.
- [ ] Reduced motion respected.
- [ ] Playwright covers the main flow against the MSW mocks.
