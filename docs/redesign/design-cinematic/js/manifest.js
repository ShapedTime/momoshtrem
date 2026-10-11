/* Every screen and state, used by screens.html and by the PNG export script. */
(function () {
  const S = (id, title, job, states, opts = {}) => ({ id, title, job, states: states.map(([key, label, route, note]) => ({ key, label, route, note })), tablet: !!opts.tablet, notes: opts.notes || '' });
  const screens = [
    S('signin', '1 · Sign in', 'Get in, and let the password manager do the work.', [
      ['default', 'Default', '/signin'], ['error', 'Wrong password', '/signin?state=error'], ['ratelimited', 'Rate-limited', '/signin?state=ratelimited']]),
    S('home', '2 · For You', 'What should I watch tonight? What\'s next, and is it ready?', [
      ['default', 'Personal', '/home?state=default'], ['trakt-off', 'Trakt not connected', '/home?state=trakt-off'], ['refreshing', 'Refreshing picks', '/home?state=refreshing'], ['syncing', 'First sync', '/home?state=syncing'], ['empty', 'First time', '/home?state=empty']], { tablet: true }),
    S('ask', '3 · Ask momoshtrem', 'Conversational discovery with visible progress and a reason per pick.', [
      ['empty', 'Start', '/ask?state=empty'], ['working', 'Working', '/ask?state=working'], ['results', 'Results', '/ask?state=results'], ['error', 'Budget reached', '/ask?state=error']], { tablet: true }),
    S('discover', '4 · Discover', 'Browse with real filters; state lives in the URL.', [
      ['default', 'Default', '/discover'], ['filtered', 'Filtered', '/discover?state=filtered'], ['filters-sheet', 'Filters sheet', '/discover?state=filtered&sheet=discover-filters'], ['empty', 'No matches', '/discover?state=empty'], ['loading', 'Loading', '/discover?state=loading']]),
    S('search', '5 · Search + ⌘K', 'Find a title, something in the library, or an action.', [
      ['idle', 'Idle', '/search?state=idle'], ['results', 'Results', '/search?state=results&q=frieren'], ['none', 'No results', '/search?state=none&q=zzxq'], ['palette', '⌘K palette', '/home?palette=1&q=sev']]),
    S('movie', '6 · Movie detail', 'One primary action that moves through four states.', [
      ['none', '1 · Add to library', '/title/movie/pastlives?state=none'], ['needs_torrent', '2 · Find a torrent', '/title/movie/perfectdays?state=needs_torrent'], ['downloading', '3 · Downloading', '/title/movie/interstellar?state=downloading'], ['ready', '4 · Ready in Infuse', '/title/movie/dune2?state=ready'], ['howto', 'How to play sheet', '/title/movie/dune2?state=ready&sheet=howto']], { tablet: true }),
    S('show', '7 · Show detail', 'Seasons incl. Specials, per-episode state, long seasons that stay short.', [
      ['frieren', 'Needs review + missing', '/title/tv/frieren'], ['severance', 'Next up highlighted', '/title/tv/severance'], ['onepiece', '1,000+ episodes', '/title/tv/onepiece'], ['specials', 'Specials', '/title/tv/doctorwho?season=0'], ['not-in-library', 'Not in library', '/title/tv/goodplace?state=none'], ['settings', 'Show settings', '/title/tv/frieren?sheet=show-settings']], { tablet: true }),
    S('torrent', '8 · Torrent search', 'Find a good torrent fast and confidently, on a phone.', [
      ['results', 'Results + best match', '/title/tv/frieren?sheet=torrent&show=frieren&se=S01E29'], ['season', 'Season pack scope', '/title/tv/frieren?sheet=torrent&show=frieren&se=S01E29&scope=season'], ['searching', 'Searching', '/title/tv/frieren?sheet=torrent&show=frieren&se=S01E29&state=searching'], ['partial', 'Partial failure', '/title/tv/severance?sheet=torrent&show=severance&se=S02E08&state=partial'], ['filtered-empty', 'Filters hide all', '/title/tv/frieren?sheet=torrent&show=frieren&se=S01E29&state=filtered-empty'], ['none', 'No results', '/title/tv/frieren?sheet=torrent&show=frieren&se=S01E29&state=none'], ['movie', 'Movie (CAM warning)', '/title/movie/perfectdays?sheet=torrent&movie=perfectdays']], { tablet: true }),
    S('mapping', '9 · Mapping review', 'Review the proposed mapping and correct it.', [
      ['progress', 'Progress', '/mapping?state=progress'], ['review', 'Review', '/mapping?state=review'], ['picker', 'Episode picker', '/mapping?state=review&sheet=ep-picker'], ['applied', 'Applied', '/mapping?state=applied']], { tablet: true }),
    S('library', '10 · Library', 'Everything you have, with status visible without hover.', [
      ['default', 'Grid', '/library'], ['list', 'List', '/library?state=list'], ['filtered-empty', 'Nothing needs review', '/library?state=filtered-empty'], ['empty', 'Empty', '/library?state=empty'], ['loading', 'Loading', '/library?state=loading']]),
    S('activity', '11 · Activity', 'Is everything working?', [
      ['default', 'Overview', '/activity'], ['detail', 'Torrent detail', '/activity/a1f3%E2%80%A69c2e'], ['empty', 'Nothing active', '/activity?state=empty'], ['offline', 'Backend offline', '/activity?state=offline']], { tablet: true }),
    S('settings', '12 · Settings', 'Integrations, AI, quality profile, playback, system status.', [
      ['trakt-off', 'Trakt · connect', '/settings?section=trakt&state=off'], ['trakt-code', 'Trakt · device code', '/settings?section=trakt&state=code'], ['trakt-waiting', 'Trakt · waiting', '/settings?section=trakt&state=waiting'], ['trakt-connected', 'Trakt · connected', '/settings?section=trakt&state=connected'], ['ai', 'AI', '/settings?section=ai'], ['quality', 'Quality profile', '/settings?section=quality'], ['playback', 'Playback', '/settings?section=playback'], ['status', 'System status', '/settings?section=status']]),
    S('system', '13 · System states', 'Skeletons, empty states, errors, toasts, dialogs, progress.', [
      ['gallery', 'Feedback patterns', '/states'], ['offline', 'Backend offline', '/offline'], ['notfound', 'Not found', '/not-found'], ['dialog', 'Confirm dialog', '/library?dialog=remove'], ['toast', 'Ready toast', '/home?toast=ready']]),
  ];
  const flows = [
    { id: 'flow1', title: 'Phone · Up next → Ready', device: 390, start: '/home?reset=1&state=default', steps: ['For You: the Frieren card in Up next says "Needs a torrent". Tap Find.', 'Torrent search opens pre-filled ("Frieren 29"), scope Episode. The banner suggests a pack.', 'Tap Inspect on the best match: a dry run shows how 16 files will map.', 'Tap "Add & review mapping".', 'Mapping review: progress, then 2 rows need review. Fix them, tap Apply.', 'Back on For You: a toast says the episodes are ready, and the card now reads "Ready to watch".'] },
    { id: 'flow2', title: 'Desktop · Ask → Find a torrent', device: 1440, start: '/ask?reset=1&q=something%20like%20Severance%20but%20lighter', steps: ['Ask momoshtrem streams its steps, then the picks.', 'Open The Good Place.', 'The primary action says "Add to library". Add it.', 'The primary action becomes "Find a torrent". The torrent panel opens with Season pack scope.'] },
    { id: 'flow3', title: 'Settings · Connect Trakt', device: 390, start: '/settings?reset=1&section=trakt&state=off', steps: ['Settings › Trakt: Connect.', 'A device code appears with "go to trakt.tv/activate".', 'Approve on Trakt (the prototype continues for you); a waiting state polls.', 'Connected: For You shows the first sync, then fills with personal picks.'] },
  ];
  window.MS = window.MS || {};
  window.MS.manifest = { screens, flows };
})();
