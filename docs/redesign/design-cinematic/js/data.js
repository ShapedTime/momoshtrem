/* Fixture data for the prototype. Shapes follow PLAN.md §6 where one exists. */
(function () {
  const T = {
    // ---- Shows ------------------------------------------------------------
    severance: { id: 'severance', type: 'tv', title: 'Severance', year: 2022, years: '2022–', rating: 8.7, genres: ['Drama', 'Mystery', 'Sci-Fi'], runtime: '~50 min', network: 'Apple TV+', status: 'Returning', creator: 'Dan Erickson',
      cast: ['Adam Scott', 'Britt Lower', 'Zach Cherry', 'Tramell Tillman', 'John Turturro', 'Patricia Arquette'],
      overview: 'Mark leads a team of office workers whose memories have been surgically divided between their work and personal lives. A mysterious colleague appears outside of work, and the truth about their jobs begins to unravel.',
      seasons: [{ n: 1, eps: 9 }, { n: 2, eps: 10 }], lib: 'partial', ready: 14, total: 19, watched: 13 },
    frieren: { id: 'frieren', type: 'tv', title: "Frieren: Beyond Journey's End", short: 'Frieren', year: 2023, years: '2023–', rating: 8.9, genres: ['Animation', 'Adventure', 'Fantasy'], runtime: '24 min', network: 'Nippon TV', status: 'Returning', creator: 'Keiichirō Saitō', anime: true,
      cast: ['Atsumi Tanezaki', 'Kana Ichinose', 'Chiaki Kobayashi', 'Hiroaki Hirata'],
      overview: 'The adventure is over, but life goes on for an elf mage just beginning to learn what living is all about. Frieren sets out on a new journey to understand the people she outlived.',
      seasons: [{ n: 0, eps: 6, name: 'Specials' }, { n: 1, eps: 38 }], lib: 'needs_review', ready: 28, total: 38, watched: 28 },
    onepiece: { id: 'onepiece', type: 'tv', title: 'One Piece', year: 1999, years: '1999–', rating: 8.7, genres: ['Animation', 'Action', 'Adventure'], runtime: '24 min', network: 'Fuji TV', status: 'Returning', creator: 'Eiichiro Oda', anime: true,
      cast: ['Mayumi Tanaka', 'Kazuya Nakai', 'Akemi Okamura', 'Kappei Yamaguchi'],
      overview: 'Monkey D. Luffy sets off with his crew to find the legendary treasure One Piece and become King of the Pirates.',
      seasons: [{ n: 0, eps: 42, name: 'Specials' }, { n: 21, eps: 156, name: 'Wano Country' }, { n: 22, eps: 38, name: 'Egghead' }], lib: 'partial', ready: 1118, total: 1123, watched: 1110, absolute: true },
    thebear: { id: 'thebear', type: 'tv', title: 'The Bear', year: 2022, years: '2022–', rating: 8.4, genres: ['Drama', 'Comedy'], runtime: '~32 min', network: 'FX', status: 'Returning', creator: 'Christopher Storer',
      cast: ['Jeremy Allen White', 'Ayo Edebiri', 'Ebon Moss-Bachrach'], overview: 'A young chef from the fine dining world returns to Chicago to run his family sandwich shop.',
      seasons: [{ n: 1, eps: 8 }, { n: 2, eps: 10 }, { n: 3, eps: 10 }, { n: 4, eps: 10 }], lib: 'ready', ready: 38, total: 38, watched: 30 },
    shogun: { id: 'shogun', type: 'tv', title: 'Shōgun', year: 2024, years: '2024–', rating: 8.6, genres: ['Drama', 'History', 'War'], runtime: '~60 min', network: 'FX', status: 'Returning', creator: 'Rachel Kondo, Justin Marks',
      cast: ['Hiroyuki Sanada', 'Cosmo Jarvis', 'Anna Sawai'], overview: 'In 1600 Japan, Lord Yoshii Toranaga fights for his life as his enemies on the Council of Regents unite against him.',
      seasons: [{ n: 1, eps: 10 }], lib: 'downloading', ready: 6, total: 10, watched: 4 },
    doctorwho: { id: 'doctorwho', type: 'tv', title: 'Doctor Who', year: 2005, years: '2005–2022', rating: 7.5, genres: ['Sci-Fi', 'Adventure', 'Drama'], runtime: '~45 min', network: 'BBC One', status: 'Ended',
      cast: ['David Tennant', 'Matt Smith', 'Jodie Whittaker'], overview: 'The Doctor, a Time Lord, travels through time and space in the TARDIS with companions.',
      seasons: [{ n: 0, eps: 160, name: 'Specials' }, { n: 1, eps: 13 }, { n: 2, eps: 13 }, { n: 3, eps: 13 }, { n: 4, eps: 13 }], lib: 'partial', ready: 40, total: 52, watched: 26 },
    bluey: { id: 'bluey', type: 'tv', title: 'Bluey', year: 2018, years: '2018–', rating: 8.9, genres: ['Animation', 'Family', 'Kids'], runtime: '7 min', network: 'ABC Kids', status: 'Returning',
      cast: ['Dave McCormack', 'Melanie Zanetti'], overview: 'Bluey is an inexhaustible six-year-old Blue Heeler dog who loves to play.',
      seasons: [{ n: 1, eps: 52 }, { n: 2, eps: 52 }, { n: 3, eps: 49 }], lib: 'ready', ready: 153, total: 153, watched: 120 },
    pluribus: { id: 'pluribus', type: 'tv', title: 'Pluribus', year: 2025, years: '2025–', rating: 8.2, genres: ['Sci-Fi', 'Drama'], runtime: '~55 min', network: 'Apple TV+', status: 'Returning', creator: 'Vince Gilligan',
      cast: ['Rhea Seehorn', 'Karolina Wydra', 'Carlos-Manuel Vesga'], overview: 'The most miserable person on Earth must save the world from happiness.',
      seasons: [{ n: 1, eps: 9 }], lib: 'none' },
    goodplace: { id: 'goodplace', type: 'tv', title: 'The Good Place', year: 2016, years: '2016–2020', rating: 8.2, genres: ['Comedy', 'Fantasy'], runtime: '22 min', network: 'NBC', status: 'Ended', creator: 'Michael Schur',
      cast: ['Kristen Bell', 'Ted Danson', 'William Jackson Harper', 'Jameela Jamil', 'D\'Arcy Carden', 'Manny Jacinto'],
      overview: 'Eleanor Shellstrop wakes up in the afterlife and is told she\'s in the Good Place. There\'s just one problem: she\'s not supposed to be there.',
      seasons: [{ n: 1, eps: 13 }, { n: 2, eps: 13 }, { n: 3, eps: 13 }, { n: 4, eps: 14 }], lib: 'none' },
    shrinking: { id: 'shrinking', type: 'tv', title: 'Shrinking', year: 2023, years: '2023–', rating: 8.0, genres: ['Comedy', 'Drama'], runtime: '~33 min', network: 'Apple TV+', lib: 'none' },
    onlymurders: { id: 'onlymurders', type: 'tv', title: 'Only Murders in the Building', year: 2021, years: '2021–', rating: 8.1, genres: ['Comedy', 'Mystery'], runtime: '~33 min', network: 'Hulu', lib: 'none' },
    mythicquest: { id: 'mythicquest', type: 'tv', title: 'Mythic Quest', year: 2020, years: '2020–2025', rating: 7.6, genres: ['Comedy'], runtime: '~30 min', network: 'Apple TV+', lib: 'none' },
    pushingdaisies: { id: 'pushingdaisies', type: 'tv', title: 'Pushing Daisies', year: 2007, years: '2007–2009', rating: 7.9, genres: ['Comedy', 'Fantasy', 'Mystery'], runtime: '42 min', network: 'ABC', lib: 'none' },
    dandadan: { id: 'dandadan', type: 'tv', title: 'Dan Da Dan', year: 2024, rating: 8.5, genres: ['Animation', 'Action', 'Comedy'], anime: true, lib: 'none' },
    mushishi: { id: 'mushishi', type: 'tv', title: 'Mushishi', year: 2005, rating: 8.6, genres: ['Animation', 'Mystery', 'Fantasy'], anime: true, lib: 'none' },
    silo: { id: 'silo', type: 'tv', title: 'Silo', year: 2023, rating: 8.1, genres: ['Sci-Fi', 'Drama'], lib: 'ready', ready: 20, total: 20, watched: 20 },
    andor: { id: 'andor', type: 'tv', title: 'Andor', year: 2022, rating: 8.4, genres: ['Sci-Fi', 'Drama'], lib: 'none' },
    slowhorses: { id: 'slowhorses', type: 'tv', title: 'Slow Horses', year: 2022, rating: 8.2, genres: ['Drama', 'Thriller'], lib: 'none' },
    studio: { id: 'studio', type: 'tv', title: 'The Studio', year: 2025, rating: 8.0, genres: ['Comedy'], lib: 'none' },
    // ---- Movies -----------------------------------------------------------
    dune2: { id: 'dune2', type: 'movie', title: 'Dune: Part Two', year: 2024, rating: 8.2, genres: ['Sci-Fi', 'Adventure'], runtime: '2h 46m', director: 'Denis Villeneuve', cast: ['Timothée Chalamet', 'Zendaya', 'Rebecca Ferguson', 'Javier Bardem', 'Austin Butler'],
      overview: 'Paul Atreides unites with Chani and the Fremen while seeking revenge against the conspirators who destroyed his family.', lib: 'ready', size: '18.4 GB', quality: '2160p' },
    interstellar: { id: 'interstellar', type: 'movie', title: 'Interstellar', year: 2014, rating: 8.5, genres: ['Sci-Fi', 'Drama', 'Adventure'], runtime: '2h 49m', director: 'Christopher Nolan', cast: ['Matthew McConaughey', 'Anne Hathaway', 'Jessica Chastain', 'Michael Caine'],
      overview: 'A team of explorers travel through a wormhole in space in an attempt to ensure humanity\'s survival.', lib: 'downloading', progress: 0.42 },
    perfectdays: { id: 'perfectdays', type: 'movie', title: 'Perfect Days', year: 2023, rating: 7.8, genres: ['Drama'], runtime: '2h 4m', director: 'Wim Wenders', cast: ['Kōji Yakusho', 'Tokio Emoto', 'Arisa Nakano'],
      overview: 'Hirayama cleans public toilets in Tokyo and seems utterly content with his simple, structured life, until a series of encounters reveal more of his past.', lib: 'needs_torrent' },
    pastlives: { id: 'pastlives', type: 'movie', title: 'Past Lives', year: 2023, rating: 7.8, genres: ['Drama', 'Romance'], runtime: '1h 46m', director: 'Celine Song', cast: ['Greta Lee', 'Teo Yoo', 'John Magaro'],
      overview: 'Nora and Hae Sung, two deeply connected childhood friends, are wrest apart after Nora\'s family emigrates from South Korea. Two decades later, they are reunited for one fateful week.', lib: 'none' },
    arrival: { id: 'arrival', type: 'movie', title: 'Arrival', year: 2016, rating: 7.6, genres: ['Sci-Fi', 'Drama'], runtime: '1h 56m', lib: 'ready' },
    aftersun: { id: 'aftersun', type: 'movie', title: 'Aftersun', year: 2022, rating: 7.6, genres: ['Drama'], runtime: '1h 42m', lib: 'none' },
    paprika: { id: 'paprika', type: 'movie', title: 'Paprika', year: 2006, rating: 7.7, genres: ['Animation', 'Sci-Fi'], runtime: '1h 30m', anime: true, lib: 'none' },
    columbus: { id: 'columbus', type: 'movie', title: 'Columbus', year: 2017, rating: 7.2, genres: ['Drama'], runtime: '1h 44m', lib: 'none' },
    totoro: { id: 'totoro', type: 'movie', title: 'My Neighbor Totoro', year: 1988, rating: 8.1, genres: ['Animation', 'Family', 'Fantasy'], runtime: '1h 26m', anime: true, lib: 'ready' },
  };
  Object.values(T).forEach((t) => { t.art = t.id; t.short = t.short || t.title; });

  // Up next (GET /me/up-next)
  const upNext = [
    { show: 'frieren', se: 'S01E29', name: 'Let\'s Go Somewhere We Can\'t Reach', availability: 'needs_torrent', progress: '28 of 38 watched', note: 'Releases call this S02E01' },
    { show: 'severance', se: 'S02E05', name: 'Trojan\'s Horse', availability: 'ready', progress: '13 of 19 watched' },
    { show: 'shogun', se: 'S01E05', name: 'Broken to the Fist', availability: 'downloading', pct: 42, progress: '4 of 10 watched' },
    { show: 'thebear', se: 'S04E01', name: 'Groundhog', availability: 'ready', progress: '30 of 38 watched' },
    { show: 'pluribus', se: 'S01E01', name: 'We Is Us', availability: 'not_in_library', progress: 'On your watchlist' },
    { show: 'onepiece', se: 'S22E39', name: 'Episode 1124', abs: 1124, availability: 'airs_on', airs: 'Airs Sun', progress: '1,110 of 1,123 watched' },
  ];

  const newEpisodes = [
    { show: 'onepiece', se: 'S22E38', name: 'The Bonds Between Giants', abs: 1123, when: 'Today', availability: 'needs_torrent' },
    { show: 'pluribus', se: 'S01E07', name: 'The Gift', when: 'Fri', availability: 'not_in_library' },
    { show: 'thebear', se: 'S04E10', name: 'Goodbye', when: 'Yesterday', availability: 'ready' },
    { show: 'bluey', se: 'S03E49', name: 'The Sign', when: 'Mon', availability: 'ready' },
  ];

  // Recommendation shelves (GET /recommendations/home)
  const shelves = [
    { slug: 'because-severance', title: 'Because you loved Severance', subtitle: 'Workplace dread, slow-burn mysteries, impeccable design', items: [
      { id: 'silo', reason: 'Same sealed-world mystery, unspools one clue an episode' },
      { id: 'andor', reason: 'Slow, precise, and pays off everything it sets up' },
      { id: 'pluribus', reason: 'Vince Gilligan\'s eerie new one; your top-rated genre' },
      { id: 'slowhorses', reason: 'Dry office politics, but spies' },
      { id: 'mythicquest', reason: 'Office comedy for when you need lighter' },
      { id: 'studio', reason: 'Long takes and workplace satire you rated highly' },
    ] },
    { slug: 'new-this-month', title: 'New this month for you', subtitle: 'Fresh releases matched to your taste', items: [
      { id: 'pluribus', reason: 'Premiered Nov 7; 94% of critics loved it' },
      { id: 'studio', reason: 'Season 2 is out; you finished S1 in two days' },
      { id: 'dandadan', reason: 'Season 2 landed; high energy, high score' },
      { id: 'aftersun', reason: 'Just added to streaming; quiet like Perfect Days' },
      { id: 'shrinking', reason: 'Season 3 started; warm and funny' },
    ] },
    { slug: 'anime', title: 'Anime you\'ll like', subtitle: 'Because of Frieren and Mushishi', items: [
      { id: 'mushishi', reason: 'The calm, episodic wonder you liked in Frieren' },
      { id: 'dandadan', reason: 'The opposite energy, but the same craft' },
      { id: 'paprika', reason: 'A 90-minute dream; Satoshi Kon at his best' },
      { id: 'totoro', reason: 'Already in your library and still unwatched' },
    ] },
    { slug: 'hidden-gems', title: 'Hidden gems', subtitle: 'Under 50k votes, over 7.5', items: [
      { id: 'columbus', reason: 'Architecture as emotion; a sibling to Past Lives' },
      { id: 'aftersun', reason: 'A small film that lands enormous' },
      { id: 'mushishi', reason: 'Beloved and barely known outside anime' },
      { id: 'pushingdaisies', reason: 'Whimsical, gone too soon, complete in 2 seasons' },
    ] },
    { slug: 'coming-soon', title: 'Coming soon', subtitle: 'On your radar, not out yet', items: [
      { id: 'severance', reason: 'Season 3 in production', badge: '2026' },
      { id: 'thebear', reason: 'Season 5 confirmed', badge: 'Summer' },
      { id: 'frieren', reason: 'Season 2 starts in January', badge: 'Jan 16' },
    ] },
  ];

  const watchlist = ['pluribus', 'pastlives', 'andor', 'goodplace', 'aftersun', 'paprika'];
  const trending = ['pluribus', 'studio', 'dune2', 'andor', 'shrinking', 'dandadan', 'slowhorses', 'pastlives'];

  // Episodes for Frieren S01 (38 episodes)
  const frierenNames = ['The Journey\'s End', 'It Didn\'t Have to Be Magic...', 'Killing Magic', 'The Land Where Souls Rest', 'Phantoms of the Dead', 'The Hero of the Village', 'Like a Fairy Tale', 'Frieren the Slayer', 'Aura the Guillotine', 'A Powerful Mage', 'Winter in the Northern Lands', 'A Real Hero', 'Aversion to One\'s Own Kind', 'Privilege of the Young', 'Smells Like Trouble', 'Long-Lived Friends', 'Take Care', 'First-Class Mage Exam', 'Well-Laid Plans', 'Necessary Killing', 'The World of Magic', 'Future Enemies', 'Conquering the Labyrinth', 'Perfect Duplicates', 'A Fatal Vulnerability', 'The Height of Magic', 'An Ordinary Mage', 'It Would Be Embarrassing When We Met Again', 'Let\'s Go Somewhere We Can\'t Reach', 'Mimic', 'Ghosts of the Past', 'The Path Ahead', 'Unforgettable Words', 'Sein\'s Choice', 'The Hero of the South', 'Battle at the Fortress', 'A Mage\'s Promise', 'Journey\'s Continuation'];
  const frierenEps = frierenNames.map((name, i) => {
    const n = i + 1;
    let state = n <= 28 ? 'watched' : 'missing';
    if (n === 27) state = 'watched';
    return { n, name, state, runtime: 24, aired: n <= 38 ? `${['Sep', 'Oct', 'Nov', 'Dec', 'Jan', 'Feb', 'Mar'][Math.floor((n - 1) / 6)] } ${((n * 7) % 28) + 1}` : '' };
  });
  // a couple of ready-but-unwatched + review
  frierenEps[27].state = 'ready';     // E28 ready, (but watched count says 28) – mark E28 ready unwatched
  frierenEps[26].state = 'watched';
  frierenEps[25].state = 'review';    // E26 needs review

  const severanceS2 = ['Hello, Ms. Cobel', 'Goodbye, Mrs. Selvig', 'Who Is Alive?', 'Woe\'s Hollow', 'Trojan\'s Horse', 'Attila', 'Chikhai Bardo', 'Sweet Vitriol', 'The After Hours', 'Cold Harbor']
    .map((name, i) => ({ n: i + 1, name, state: i < 4 ? 'watched' : i < 6 ? 'ready' : i === 6 ? 'downloading' : i === 9 ? 'upcoming' : 'missing', pct: i === 6 ? 64 : undefined, aired: i === 9 ? 'Airs Mar 21' : `Jan ${17 + i * 7 > 31 ? '' : 17 + i * 7}` }));

  // Torrent search (GET /search/torrents)
  const releases = [
    { id: 'r1', raw: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]', size: '13.2 GB', sizeB: 13.2, seeders: 412, leechers: 18, age: '9 mo', indexer: 'Nyaa', attrs: ['1080p', 'WEB', 'x264', 'SubsPlease', 'Episodes 29–38'], pack: 'range', score: 96, best: true,
      why: ['Covers all 10 missing episodes', '1080p WEB from SubsPlease', '412 seeders', '1.3 GB per episode fits your profile'] },
    { id: 'r2', raw: 'Frieren.Beyond.Journeys.End.S02.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG', size: '14.1 GB', sizeB: 14.1, seeders: 188, leechers: 9, age: '9 mo', indexer: 'TorrentLeech', attrs: ['1080p', 'WEB-DL', 'H.264', 'AAC 2.0', 'VARYG', 'Season 2 pack'], pack: 'season', score: 91, note: 'Scene "Season 2" = TMDB S01E29–E38; mapped automatically' },
    { id: 'r3', raw: '[Erai-raws] Sousou no Frieren - 29 [1080p][Multiple Subtitle][7B1C0F1E].mkv', size: '1.4 GB', sizeB: 1.4, seeders: 96, leechers: 2, age: '9 mo', indexer: 'Nyaa', attrs: ['1080p', 'WEB', 'Multi-sub', 'Erai-raws'], pack: 'episode', score: 84 },
    { id: 'r4', raw: 'Frieren.Beyond.Journeys.End.S01.COMPLETE.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.5.1-ANiHLS', size: '186.0 GB', sizeB: 186, seeders: 31, leechers: 44, age: '2 mo', indexer: 'TorrentLeech', attrs: ['2160p', 'BluRay REMUX', 'HEVC', 'DTS-HD MA 5.1', 'ANiHLS', 'Complete'], pack: 'complete', score: 62, flags: ['size_outlier'], warn: 'Over your 8 GB/hour limit' },
    { id: 'r5', raw: '[ASW] Sousou no Frieren - 29 [1080p HEVC x265 10Bit][AAC]', size: '312 MB', sizeB: 0.31, seeders: 64, leechers: 1, age: '9 mo', indexer: 'Nyaa', attrs: ['1080p', 'x265', '10-bit', 'ASW'], pack: 'episode', score: 80 },
    { id: 'r6', raw: 'Frieren - 29 [720p] [Judas]', size: '220 MB', sizeB: 0.22, seeders: 0, leechers: 0, age: '8 mo', indexer: 'The Pirate Bay', attrs: ['720p', 'x265', 'Judas'], pack: 'episode', score: 21, flags: ['no_seeders'], warn: 'No seeders' },
  ];
  const facets = {
    resolution: { '2160p': 4, '1080p': 31, '720p': 8, 'Unknown': 2 },
    source: { 'WEB-DL': 18, 'WEB': 14, 'BluRay': 7, 'REMUX': 2, 'HDTV': 2 },
    codec: { 'x265 / HEVC': 16, 'x264 / AVC': 25, 'AV1': 2 },
    hdr: { 'HDR10': 4, 'Dolby Vision': 1, 'SDR': 40 },
    audio: { 'AAC': 29, 'DDP 5.1': 6, 'Atmos': 1, 'DTS-HD MA': 2, 'FLAC': 3 },
    pack: { 'Episode': 27, 'Episode range': 5, 'Season': 9, 'Complete': 4 },
    indexer: { 'Nyaa': 26, 'TorrentLeech': 11, 'The Pirate Bay': 8 },
    group: { 'SubsPlease': 12, 'Erai-raws': 9, 'VARYG': 4, 'ASW': 6, 'Judas': 3, 'ANiHLS': 2 },
  };

  // Severance release for movie/episode context
  const severanceRelease = 'Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX';

  // Mapping job (GET /assignment-jobs/{id})
  const mapping = {
    torrent: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]',
    show: 'frieren',
    groups: {
      matched: [29, 30, 31, 32, 33, 34, 36, 37, 38].map((n) => ({ id: `m${n}`, file: `[SubsPlease] Sousou no Frieren - ${n} (1080p) [${(0x8E3D5C21 + n * 977).toString(16).toUpperCase().slice(0, 8)}].mkv`, size: `${(1.28 + (n % 3) * 0.04).toFixed(2)} GB`, target: { s: 1, e: n, name: frierenNames[n - 1] }, conf: 'high', source: 'rules', reason: `Absolute #${n} → S01E${String(n).padStart(2, '0')} (single-season show)` })),
      needs_review: [
        { id: 'nr1', file: '[SubsPlease] Sousou no Frieren - 35v2 (1080p) [5A1B3C9D].mkv', size: '1.31 GB', target: { s: 1, e: 35, name: frierenNames[34] }, conf: 'medium', source: 'ai', reason: 'AI: "v2" is a re-release of #35; the v1 file is already assigned', conflict: 'Replaces the current S01E35 file' },
        { id: 'nr2', file: '[SubsPlease] Sousou no Frieren - 28.5 (1080p) [0C4E9A12].mkv', size: '640 MB', target: { s: 0, e: 3, name: 'Recap: The Journey So Far' }, conf: 'low', source: 'ai', reason: 'AI: ".5" usually marks a recap special; matched S00E03 by air date (Mar 15)' },
      ],
      specials: [
        { id: 'sp1', file: '[SubsPlease] Sousou no Frieren - Mini Anime 05 (1080p) [AB12CD34].mkv', size: '58 MB', target: { s: 0, e: 6, name: '“Frieren” Mini Anime 5' }, conf: 'high', source: 'ai', reason: 'AI: "Mini Anime 05" → S00E06 by title' },
      ],
      extras: [
        { id: 'ex1', file: '[SubsPlease] Sousou no Frieren - NCOP2 (1080p).mkv', size: '142 MB', kind: 'Opening (creditless)', reason: 'NCOP = creditless opening; ignored by default' },
        { id: 'ex2', file: '[SubsPlease] Sousou no Frieren - NCED2 (1080p).mkv', size: '139 MB', kind: 'Ending (creditless)', reason: 'NCED = creditless ending' },
        { id: 'ex3', file: 'Fonts/OpenSans-Semibold.ttf', size: '220 KB', kind: 'Font', reason: 'Not a video' },
      ],
      unmatched: [
        { id: 'un1', file: '[SubsPlease] Sousou no Frieren - Special Extra (1080p) [F00DBABE].mkv', size: '210 MB', target: null, conf: 'none', source: 'rules', reason: 'No number and no special title matched. Pick an episode or ignore it.' },
      ],
    },
  };

  // Activity (GET /torrents)
  const torrents = [
    { hash: 'a1f3…9c2e', name: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]', title: 'frieren', label: 'Frieren · S01E29–E38', state: 'downloading', pct: 64, down: '18.4 MB/s', up: '1.2 MB/s', peers: 47, seeds: 212, eta: '4 min', size: '13.2 GB', health: 'good', streaming: false },
    { hash: '7b20…11fa', name: 'Shogun.2024.S01.2160p.DSNP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', title: 'shogun', label: 'Shōgun · Season 1', state: 'streaming', pct: 61, down: '9.1 MB/s', up: '420 KB/s', peers: 23, seeds: 140, size: '42.6 GB', health: 'good', streaming: { who: 'Apple TV · Infuse', file: 'S01E05 Broken to the Fist', at: '18:42 / 58:10', buffer: 'Buffered 6 min ahead' } },
    { hash: 'c9d4…a7b3', name: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT', title: 'interstellar', label: 'Interstellar', state: 'downloading', pct: 42, down: '2.3 MB/s', up: '80 KB/s', peers: 6, seeds: 9, eta: '1 h 52 min', size: '63.1 GB', health: 'weak', streaming: false },
    { hash: '0e55…3d1c', name: 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', title: 'severance', label: 'Severance · Season 2', state: 'paused', pct: 70, down: '—', up: '—', peers: 0, seeds: 0, size: '58.3 GB', health: 'paused' },
    { hash: '4f1a…be90', name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', title: 'dune2', label: 'Dune: Part Two', state: 'idle', pct: 100, size: '18.4 GB', health: 'idle', lastPlayed: 'Played 3 days ago' },
    { hash: '9a02…77e4', name: 'Bluey.S03.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb', title: 'bluey', label: 'Bluey · Season 3', state: 'idle', pct: 100, size: '21.7 GB', health: 'idle', lastPlayed: 'Played yesterday' },
    { hash: 'e3c8…0d4f', name: 'The.Bear.S04.1080p.HULU.WEB-DL.DDP5.1.H.264-NTb', title: 'thebear', label: 'The Bear · Season 4', state: 'idle', pct: 100, size: '17.9 GB', health: 'idle', lastPlayed: 'Played 5 days ago' },
  ];

  // Ask momoshtrem
  const askResults = [
    { id: 'goodplace', reason: 'Same "what is this place really?" mystery as Severance, played for big laughs. Bright, kind and fast, with twists that land every few episodes.', match: 'Mystery + workplace · lighter' },
    { id: 'mythicquest', reason: 'An office comedy about a game studio. Same absurd corporate culture, none of the dread.', match: 'Office satire' },
    { id: 'onlymurders', reason: 'A cosy whodunnit with a season-long puzzle, and you rated Knives Out 9/10.', match: 'Puzzle-box · cosy' },
    { id: 'shrinking', reason: 'Warm, funny and about grief and work. You finished The Bear and Ted Lasso.', match: 'Warm · 30-min episodes' },
    { id: 'pushingdaisies', reason: 'Candy-coloured mystery with Severance-level design. Two seasons, complete story.', match: 'Stylised · complete' },
  ];
  const askThreads = [
    { id: 't1', title: 'Something like Severance but lighter', when: 'Now' },
    { id: 't2', title: 'A 90-minute thriller for tonight', when: 'Yesterday' },
    { id: 't3', title: 'Anime with a Frieren vibe', when: 'Oct 2' },
    { id: 't4', title: 'Movies to watch with my parents', when: 'Sep 28' },
  ];

  const subtitles = {
    have: [{ lang: 'English', code: 'EN', source: 'Embedded', note: 'In the file' }, { lang: 'Russian', code: 'RU', source: 'OpenSubtitles', note: 'Matches release · 1,204 downloads' }],
    results: [
      { lang: 'Azerbaijani', file: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.az.srt', match: 'Exact release', downloads: 312 },
      { lang: 'Azerbaijani', file: 'Dune.Part.Two.2024.1080p.BluRay.x264-SPARKS.az.srt', match: 'Different cut timing', downloads: 88 },
    ],
  };

  window.MS = window.MS || {};
  window.MS.data = { T, upNext, newEpisodes, shelves, watchlist, trending, frierenEps, severanceS2, releases, facets, severanceRelease, mapping, torrents, askResults, askThreads, subtitles };
})();
