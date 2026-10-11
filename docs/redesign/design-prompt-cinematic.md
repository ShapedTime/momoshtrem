# momoshtrem: design brief (Direction A, Cinematic dark)

> Paste everything below into a new Claude Design session. Attach `docs/screenshots/*.png` (the current UI) for context only.
> A parallel session is exploring a different direction ("Playful cat-forward"). Commit fully to **Cinematic dark** here so the two can be compared.

## Visual direction: Cinematic dark

- **Mood.** A premium streaming app at night: think Apple TV app, Infuse, a cinema lobby after dark. It's content-first, so the artwork carries the colour and the chrome recedes.
- **Palette:**
  - Near-black with a very subtle cool tint (not pure `#000`) and layered surfaces.
  - Translucent "glass" panels with backdrop blur over artwork.
  - **One** signature accent, and **not Netflix red**; move away from the current red-on-black. Explore a warm "cat-eye" amber/gold or a soft luminous lavender, and show both before committing.
  - Status colours tuned for dark backgrounds.
- **Typography.** A refined grotesk or neo-grotesk (e.g. Inter Display, Geist or an SF-like face). Large, confident titles on heroes; tight tracking on display sizes; comfortable line height in body text.
- **Imagery:**
  - Full-bleed backdrops with gradient scrims.
  - Poster grids with a subtle lift on hover/focus.
  - Ambient colour sampled from the artwork, e.g. a hero glow tinted by the backdrop.
- **Motion.** Slow, smooth and cinematic: crossfades, a light parallax on the hero, shared-element transitions from poster to detail page. Everything has a reduced-motion fallback.
- **The cat.** Understated and elegant, never cartoonish. It lives in:
  - the logo;
  - glowing cat eyes in the dark as the loading indicator;
  - a silhouette in empty states;
  - a small cat avatar as the face of "Ask momoshtrem".
- **Themes.** Dark only. A light theme isn't required.

## What momoshtrem is

momoshtrem is my personal, self-hosted media hub, and I'm its only user. It's a web app I use on my **phone** (on the couch in the evening) and on my **laptop**. In it I decide what to watch, collect it into my library and get it ready. The watching itself happens on **Apple TV in Infuse**, which streams from momoshtrem over WebDAV. The web app is **not a video player**. It's the concierge and control room.

Today the app is called "PrettyTVCatalog". It's a generic dark, red-accented, Netflix-style catalog (screenshots attached for context only; don't copy it). I'm renaming it **momoshtrem** and giving it a new identity with a **little cat logo**. Redesign it end to end: identity, design system, and every key screen, on desktop and phone.

## Brand

- **Name:** always lowercase, **momoshtrem**.
- **Logo:** a little cat mark. Simple and geometric enough to read at 16 px as a favicon; friendly, never childish.
  - Show 3 explorations, then refine the strongest.
  - Final set: mark, wordmark, lockup, monochrome version, iOS app icon (1024 and 180), favicon (16/32) and a maskable PWA icon (512).
- **Mascot:** the cat is a recurring character. "Momo" is a working name; propose a better one if you like. It shows up in:
  - loading, empty, error and success states;
  - as the face and voice of the AI assistant ("Ask momoshtrem").

  How prominent the cat is depends on the visual direction above.
- **Voice:** short, warm, a little witty. Errors that matter stay clear and calm, not cute.

## The five jobs the app must nail

1. **"What should I watch tonight?"** Personal picks from my Trakt watch history, plus an AI agent that researches what's new and explains why each pick fits me.
2. **"What's next in the shows I'm watching, and is it ready?"** Up next, with availability for each show.
3. **"Get this ready."** Find a good torrent fast and confidently, often on my phone. Add it, and know when it's playable in Infuse.
4. **"Fix the mapping."** Sometimes a torrent's files don't map cleanly to episodes: anime absolute numbering, specials, odd names. I need to review the proposed mapping and correct it.
5. **"Is everything working?"** What's downloading or streaming, how healthy it is, how fast it's going.

## What's broken today (the new design must solve these)

- **Titles only appear on hover.** On a phone you can't tell what a poster is. Grids also overlap or leave uneven gaps.
- **The primary action lies.** A disabled green "Ready to Stream" button sits beside a red "Search Torrents" that stays primary even after a title is ready. Nothing explains how to actually play something (Infuse over WebDAV).
- **Torrent search falls apart on a phone:**
  - Release names get cut off.
  - Size and seeders wrap into cramped stacks.
  - The "Add" button stretches across the row and is easy to tap by accident.
  - The modal jumps in height as results load.
  - Closing it loses the query.
  - Filters that hide everything just say "No torrents found".
- **Destructive actions are loud.** "Remove" is a bigger tap target than "Subtitles".
- **Long seasons are endless scrolls.** A 38-episode anime season becomes a page about 8,000 px tall. One Piece has more than 1,000 episodes. Design for compact rows, arcs/ranges and jump-to-episode.
- **Horizontal carousels aren't discoverable** with a mouse.
- **Two competing accent colours** (red and blue), so it's unclear what's primary.

## Information architecture

- **Primary:** For You (home) · Discover · Search · Library · Activity.
- **Secondary** (in the cat/user menu): Settings, Log out.
- **Desktop:** a top bar or slim side rail, plus a ⌘K command palette for search and quick actions.
- **Phone:**
  - A bottom tab bar with 5 items and large titles.
  - Bottom sheets instead of centred modals; primary actions within thumb reach.
  - Safe-area aware, since it will be installed as a PWA on iPhone.

## Screens (each at phone 390 px and desktop 1440 px, with the states listed)

1. **Sign in**
   - **Fields:** username and password. Both must be real fields so password managers save and fill them.
   - **States:** error ("Wrong username or password") and rate-limited.
   - A small cat moment.
2. **For You (home)**
   - **Up next:** the next episode of each show I'm watching (from Trakt), each with its availability:
     - Ready to watch
     - Downloading 42%
     - Needs a torrent
     - Not in library
     - Airs Thu
   - **New episodes** for my shows.
   - **4–6 AI shelves,** e.g. "Because you loved Severance", "New this month for you", "Anime you'll like", "Hidden gems", "Coming soon".
     - Each item has a one-line reason.
     - Quick feedback: like, dislike, not interested, already seen.
   - **My Trakt watchlist.**
   - **The "Ask momoshtrem…" entry point.**
   - **States:**
     - Trakt not connected: trending plus a friendly connect call to action.
     - Picks being refreshed.
     - First-time empty.
3. **Ask momoshtrem:** conversational discovery.
   - Example asks: "something like Severance but lighter", "a 90-minute thriller for tonight", "anime with a Frieren vibe".
   - **While the agent works,** it streams visible progress steps: "Reading your history…", "Searching the web for new releases…", "Checking TMDB…".
   - **Results:** cards with a reason each.
     - Refine chips: lighter, shorter, newer, only movies, only anime.
     - Add to library from a card, plus feedback.
   - Past threads.
   - **Layout:** full-screen and chat-like on phone; a side panel or page on desktop.
4. **Discover:** browse with real filters.
   - **Filters:** type, several genres at once, year range, rating, runtime, original language or country (including an "Anime" toggle), status. Plus sort.
   - Mood chips and infinite scroll.
   - On phone, filters open in a bottom sheet.
   - Filter state persists in the URL.
5. **Search**
   - ⌘K palette on desktop: titles, my library and actions.
   - A full results page with Movies and Shows tabs.
   - A dedicated search tab on phone.
6. **Movie detail**
   - **Hero:** backdrop, poster, title, year, runtime, genres, rating, director, cast, trailer.
   - **Personal:** "Why it's recommended for you", More like this, my Trakt state (watched on …, my rating).
   - **One primary action** that moves through the states:
     1. Add to library
     2. Find a torrent
     3. Downloading / preparing
     4. Ready in Infuse, with a "copy WebDAV path" and how-to-play hint
   - **Current torrent card:** quality, size, health.
   - **Subtitles panel:** downloaded languages; search OpenSubtitles.
7. **Show detail**
   - The same hero.
   - Seasons as tabs or a segmented control, **including "Specials"**.
   - **Episode list** with per-episode state chips:
     - Watched
     - Ready
     - Downloading
     - Missing
     - Airs Oct 24
     - Needs review
   - The next-up episode is highlighted.
   - **Bulk actions:** find a season pack; find missing episodes.
   - A "mapping needs review" banner.
   - **Show settings:**
     - Episode ordering: Aired, Absolute (anime) or a TMDB episode group.
     - Include specials.
8. **Torrent search:** the most important utility, and it must be excellent on a phone. It opens from a movie, a season or an episode.
   - **Search field:** pre-filled from context (e.g. "Severance S02E05"), with a scope switch: Episode · Season pack · Any.
   - **A highlighted "Best match"** with a short why, e.g. "2160p WEB-DL · Dolby Vision · 4.1 GB · 312 seeders · matches your quality profile".
   - **Results** as rows or cards, each with:
     - The release name, which must wrap gracefully. Long dotted names like `Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX` are common. Show the exact wrapping/truncation rule.
     - Parsed attribute chips: 2160p, WEB-DL, x265, DV, Atmos, group, "Season 2 pack".
     - Size, seeders/leechers, age, indexer.
     - Warnings: CAM, no seeders, suspicious size.
     - Actions: **Add**, **Inspect** (preview the files and their episode mapping before adding), copy magnet.
   - **Filters:** resolution, source, codec, HDR/DV, audio, pack type, size range, minimum seeders, indexer, release group. Each shows facet counts.
   - **Sort and pages.** Sort by Best, Seeders, Size or Newest. Pagination shows a total ("Showing 25 of 143 · Load more").
   - **States:** searching across several indexers; no results (suggest broadening); partial failure ("2 indexers timed out").
   - **On phone:**
     - A full-height bottom sheet with a sticky search field and a row of filter chips.
     - The full filters open in a nested sheet.
     - Actions stay within thumb reach.
     - **Never any horizontal scrolling.**
9. **Episode mapping review.** Shown after adding a torrent to a show, or from Inspect.
   - **Progress:** Fetching metadata → Matching files → Asking AI about the tricky ones.
   - **Groups:** Matched · Needs review · Specials · Extras & ignored · Unmatched.
   - **Each row:**
     - The file name (wrapping) and size.
     - The target episode (S01E05, "Title"), with confidence.
     - A short reason, e.g. AI: "absolute #1071 → S21E79 by air order".
   - **Controls:**
     - An inline episode picker: searchable, grouped by season, including Specials.
     - Ignore.
     - "Re-check with AI", with an optional hint such as "these are absolute numbers".
     - "Apply 24 mappings".
   - **Layout:** two panes (files ↔ episodes) on desktop; a stacked list with a picker sheet on phone.
10. **Library**
    - Grid/list toggle, sort, and search within the library.
    - **Filters:** All · Movies · Shows · Watching · Ready · Incomplete · Needs review.
    - **Cards show status without hover** (this is used on touch):
      - ready / total episodes;
      - watched progress;
      - downloading state.
    - Remove, with confirmation.
11. **Activity:** what's happening right now.
    - Active and idle torrents, each linked to its movie or show.
    - Speeds, peers, progress, and what's streaming now.
    - Pause, resume and remove.
    - **Per-torrent detail:**
      - The file list.
      - A collapsed "Advanced" section with nerd stats: a piece map and active readers.
12. **Settings**
    - **Integrations:** Trakt. Connecting uses a device code:
      1. Show the code and "go to trakt.tv/activate".
      2. A waiting state.
      3. Connected: username, last sync, disconnect.
    - **AI:**
      - Recommendation schedule and model.
      - Monthly budget, and spend so far this month.
      - Classifier on/off.
    - **Quality profile:**
      - Preferred and max resolution; max size per hour.
      - Prefer x265 / HDR / DV.
      - Terms to avoid; preferred groups.
    - **Playback:** the WebDAV address and a short Infuse setup guide.
    - **System status:** backend, Jackett, TMDB, Trakt, AI.
13. **System states and feedback**
    - Skeleton loading.
    - Empty states: empty library, no results, Trakt not connected.
    - Errors: backend offline, indexer failures.
    - Toasts, confirmation dialogs and progress indicators.

## Realistic content to use

- **Shows:** Severance, Frieren: Beyond Journey's End, One Piece, The Bear, Shōgun, Doctor Who (with specials), Bluey, Pluribus.
- **Movies:** Dune: Part Two, Interstellar, Perfect Days, Past Lives.
- **Artwork:** TMDB-style posters (2:3), backdrops (16:9), episode stills (16:9) and cast photos. Artwork comes in every colour, so the UI must stay legible over any of it.
- **Real-looking release and file names** for torrent search and mapping review:
  - `[SubsPlease] Sousou no Frieren - 05 (1080p) [8E3D5C21].mkv`
  - `One.Piece.1071.1080p.WEB.x264-GROUP.mkv`
  - `Doctor.Who.2005.S00E05.The.Runaway.Bride.720p.mkv`
  - `Bluey.S03.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb/`

## Constraints for handoff

- **Stack:** Next.js 16 (App Router), React 19, Tailwind CSS v4 with CSS-first `@theme` tokens.
- **Tokens:** deliver them as CSS custom properties covering colour (including semantic status colours: ready, downloading, missing, needs review, watched, error, warning), type scale, spacing, radii, elevation, blur, motion durations and easings, and z-index.
- **Icons:** Lucide or an equivalent outline set.
- **Fonts:** Google Fonts, or self-hosted via `next/font`.
- **Responsive:**
  - 320 → 1920 px, with breakpoints around 640, 1024 and 1440.
  - No horizontal scrolling at 320 px.
  - Long text (release names, file paths, titles) wraps or truncates intentionally; specify the rule.
- **Touch:** targets of at least 44 px, and nothing important that only appears on hover.
- **Accessibility:**
  - WCAG AA contrast.
  - Visible focus states.
  - A reduced-motion variant of every animation.

## Deliverables

1. Three logo explorations, then the final logo set.
2. **A design system page:**
   - Colour, typography, spacing, radii, elevation, motion and iconography.
   - Components with all their states:
     - Buttons, inputs, chips and filter chips, segmented controls, tabs.
     - Cards: poster, landscape, episode, release (torrent result) and torrent activity.
     - Status pills, sheets and dialogs, toasts.
     - Skeletons, empty and error states, progress.
     - The command palette and the navigation bars.
3. Every screen above at 390 px and 1440 px (the key ones also at 768 px), including the listed states.
4. **A clickable prototype of three flows:**
   1. **Phone:** For You → an Up next item marked "Needs a torrent" → Torrent search → Inspect → Add → Mapping review → Ready.
   2. **Desktop:** Ask momoshtrem "something like Severance but lighter" → pick a result → Add to library → Find a torrent.
   3. **Settings:** Connect Trakt (device code) → first sync → For You fills with personal picks.
5. **Short handoff notes:**
   - The layout grid and the component inventory.
   - Interaction details: sheet behaviour, sticky elements, loading choreography.
   - Any data a screen needs that isn't mentioned above.
