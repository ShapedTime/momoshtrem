/* Content for the prototype. Real titles and people; episode titles only where they are known,
   otherwise "Episode N" (which is also what TMDB shows for fresh anime episodes). */
(function () {
  const T = {};
  const add = (id, o) => {
    T[id] = { id, ...o };
    ART.define(id, { title: o.title, ...o.art });
  };
  const ep = (s, n, title, o = {}) => ({ s, n, title: title || `Episode ${n}`, ...o });

  // ------------------------------------------------------------------ shows in the story
  add('severance', {
    type: 'show', title: 'Severance', year: 2022, years: '2022–', network: 'Apple TV+', rating: 8.4, runtime: 50,
    genres: ['Drama', 'Mystery', 'Sci-Fi'], creator: 'Dan Erickson', status: 'Returning series',
    overview: 'Mark leads a team at Lumon whose memories have been surgically split between work and home. Then a mysterious colleague appears outside the office, and the split starts to crack.',
    cast: [['Adam Scott', 'Mark Scout'], ['Britt Lower', 'Helly R.'], ['Zach Cherry', 'Dylan G.'], ['John Turturro', 'Irving B.'], ['Tramell Tillman', 'Seth Milchick'], ['Patricia Arquette', 'Harmony Cobel'], ['Christopher Walken', 'Burt G.']],
    trailer: '2:31',
    art: { motif: 'corridor', palette: ['#0F3E44', '#1C5E63', '#E9F1EC', '#2F7A73', '#F7FBF8'], font: 'sans', ink: '#0F3E44', titleTop: true },
    trakt: { rating: 10, progress: '15 of 19 watched', last: 'Watched S02E05 on Thu' },
    seasons: [
      { n: 1, year: 2022, eps: ['Good News About Hell', 'Half Loop', 'In Perpetuity', 'The You You Are', 'The Grim Barbarity of Optics and Design', 'Hide and Seek', 'Defiant Jazz', "What's for Dinner?", 'The We We Are']
        .map((t, i) => ep(1, i + 1, t, { state: 'watched', runtime: [57, 51, 52, 55, 56, 53, 49, 56, 40][i], date: ['Feb 18, 2022', 'Feb 18, 2022', 'Feb 25, 2022', 'Mar 4, 2022', 'Mar 11, 2022', 'Mar 18, 2022', 'Mar 25, 2022', 'Apr 1, 2022', 'Apr 8, 2022'][i] })) },
      { n: 2, year: 2025, eps: ['Hello, Ms. Cobel', 'Goodbye, Mrs. Selvig', 'Who Is Alive?', "Woe's Hollow", "Trojan's Horse", 'Attila', 'Chikhai Bardo', 'Sweet Vitriol', 'The After Hours', 'Cold Harbor']
        .map((t, i) => ep(2, i + 1, t, { state: i < 5 ? 'watched' : i < 8 ? 'ready' : i === 8 ? 'downloading' : 'missing', progress: i === 8 ? 64 : undefined, runtime: [56, 50, 49, 56, 52, 49, 54, 43, 55, 76][i], date: ['Jan 17, 2025', 'Jan 24, 2025', 'Jan 31, 2025', 'Feb 7, 2025', 'Feb 14, 2025', 'Feb 21, 2025', 'Feb 28, 2025', 'Mar 7, 2025', 'Mar 14, 2025', 'Mar 21, 2025'][i] })) },
    ],
    next: { s: 2, n: 6 },
    torrent: { name: 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', size: '41.3 GB', q: ['2160p', 'WEB-DL', 'DV', 'Atmos'], health: 'Healthy · 210 seeders', progress: 84 },
  });

  const frierenTitles = ["The Journey's End", "It Didn't Have to Be Magic...", 'Killing Magic', 'The Land Where Souls Rest', 'Phantoms of the Dead', 'The Hero of the Village', 'Like a Fairy Tale', 'Frieren the Slayer', 'Aura the Guillotine', 'A Powerful Mage', 'Winter in the Northern Lands', 'A Real Hero', 'Aversion to One’s Own Kind', 'Privilege of the Young', 'Smells Like Trouble', 'Long-Lived Friends', 'Take Care', 'First-Class Mage Exam', 'Well-Laid Plans', 'Necessary Killing', 'The World of Magic', 'Future Enemies', 'Conquering the Labyrinth', 'Perfect Replicas', 'A Fatal Vulnerability', 'The Height of Magic', 'An Ordinary Mage', 'It Would Be Embarrassing When We Met Again'];
  add('frieren', {
    type: 'show', title: "Frieren: Beyond Journey's End", short: 'Frieren', original: 'Sousou no Frieren', year: 2023, years: '2023–', network: 'Nippon TV', rating: 8.8, runtime: 24, anime: true,
    genres: ['Animation', 'Fantasy', 'Adventure'], creator: 'Keiichirō Saitō (director)', status: 'Returning series',
    overview: 'After the hero party defeats the Demon King, the elf mage Frieren outlives her friends and sets out to understand the people she travelled with, one quiet village at a time.',
    cast: [['Atsumi Tanezaki', 'Frieren'], ['Kana Ichinose', 'Fern'], ['Chiaki Kobayashi', 'Stark'], ['Nobuhiko Okamoto', 'Himmel'], ['Hiroki Tōchi', 'Heiter'], ['Yōji Ueda', 'Eisen']],
    trailer: '1:30',
    art: { motif: 'field', palette: ['#CFE3EE', '#F3EFE2', '#9BC59A', '#7FAF86', '#F2D16B'], font: 'serif', ink: '#2E4A5A', posterTitle: 'Frieren', titleTop: true },
    trakt: { rating: 10, progress: '28 of 38 watched', last: 'Watched E28 in March' },
    ordering: 'Aired', sceneNote: 'Releases call episodes 29–38 “Season 2”. TMDB keeps them in season 1.',
    seasons: [
      { n: 0, name: 'Specials', eps: [ep(0, 1, 'Frieren’s Mini Anime: 1', { state: 'missing', runtime: 3, date: 'Oct 2023' }), ep(0, 2, 'Frieren’s Mini Anime: 2', { state: 'missing', runtime: 3, date: 'Oct 2023' })] },
      { n: 1, year: 2023, eps: Array.from({ length: 38 }, (_, i) => ep(1, i + 1, frierenTitles[i], {
        state: i < 28 ? 'watched' : 'missing', runtime: 24, abs: i + 1,
        date: i < 28 ? `${['Sep 29', 'Sep 29', 'Sep 29', 'Sep 29', 'Oct 6', 'Oct 13', 'Oct 20', 'Oct 27', 'Nov 3', 'Nov 10', 'Nov 17', 'Nov 24', 'Dec 1', 'Dec 8', 'Dec 15', 'Dec 22', 'Jan 5', 'Jan 12', 'Jan 19', 'Jan 26', 'Feb 2', 'Feb 9', 'Feb 16', 'Feb 23', 'Mar 1', 'Mar 8', 'Mar 15', 'Mar 22'][i]}, ${i < 16 ? 2023 : 2024}` : `${['Jan 16', 'Jan 23', 'Jan 30', 'Feb 6', 'Feb 13', 'Feb 20', 'Feb 27', 'Mar 6', 'Mar 13', 'Mar 20'][i - 28]}, 2026`,
      })) },
    ],
    next: { s: 1, n: 29 },
  });

  const opArcs = [['East Blue', 1, 61], ['Alabasta', 62, 135], ['Sky Island', 136, 206], ['Water 7', 207, 325], ['Thriller Bark', 326, 384], ['Summit War', 385, 516], ['Fish-Man Island', 517, 574], ['Dressrosa', 575, 746], ['Whole Cake Island', 747, 891], ['Wano Country', 892, 1085], ['Egghead', 1086, 1155], ['Elbaph', 1156, 1160]];
  add('one-piece', {
    type: 'show', title: 'One Piece', year: 1999, years: '1999–', network: 'Fuji TV', rating: 8.7, runtime: 24, anime: true,
    genres: ['Animation', 'Action', 'Adventure'], creator: 'Eiichiro Oda (original story)', status: 'Returning series',
    overview: 'Monkey D. Luffy and his Straw Hat crew sail the Grand Line in search of the legendary treasure, the One Piece, and Luffy’s dream of becoming King of the Pirates.',
    cast: [['Mayumi Tanaka', 'Monkey D. Luffy'], ['Kazuya Nakai', 'Roronoa Zoro'], ['Akemi Okamura', 'Nami'], ['Kappei Yamaguchi', 'Usopp'], ['Hiroaki Hirata', 'Sanji'], ['Ikue Ōtani', 'Tony Tony Chopper']],
    art: { motif: 'sea', palette: ['#7CC6EE', '#1F5FA8', '#F2C94C', '#D63A2F', '#FFFFFF'], font: 'cond', ink: '#FFE58A' },
    trakt: { rating: 9, progress: '1,159 of 1,160 watched', last: 'Watched episode 1159 on Sunday' },
    ordering: 'Absolute', arcs: opArcs, totalEps: 1160,
    next: { abs: 1160 },
  });

  add('the-bear', {
    type: 'show', title: 'The Bear', year: 2022, years: '2022–', network: 'FX', rating: 8.1, runtime: 33,
    genres: ['Comedy', 'Drama'], creator: 'Christopher Storer', status: 'Returning series',
    overview: 'A young fine-dining chef comes home to Chicago to run his late brother’s chaotic sandwich shop, and slowly tries to turn it into something better.',
    cast: [['Jeremy Allen White', 'Carmy Berzatto'], ['Ayo Edebiri', 'Sydney Adamu'], ['Ebon Moss-Bachrach', 'Richie Jerimovich'], ['Abby Elliott', 'Natalie Berzatto'], ['Lionel Boyce', 'Marcus']],
    art: { motif: 'stripes', palette: ['#EDE6D6', '#E2D8C3', '#2C4E8A', '#F2C230', '#1F2D45'], font: 'cond', ink: '#1F2D45' },
    trakt: { rating: 9, progress: '21 of 38 watched', last: 'Watched S03E03 yesterday' },
    seasons: [
      { n: 1, year: 2022, eps: ['System', 'Hands', 'Brigade', 'Dogs', 'Sheridan', 'Ceres', 'Review', 'Braciole'].map((t, i) => ep(1, i + 1, t, { state: 'watched', runtime: 30 })) },
      { n: 2, year: 2023, eps: ['Beef', 'Pasta', 'Sundae', 'Honeydew', 'Pop', 'Fishes', 'Forks', 'Bolognese', 'Omelette', 'The Bear'].map((t, i) => ep(2, i + 1, t, { state: 'watched', runtime: 34 })) },
      { n: 3, year: 2024, eps: ['Tomorrow', 'Next', 'Doors', 'Violet', 'Children', 'Napkins', 'Legacy', 'Ice Chips', 'Apologies', 'Forever'].map((t, i) => ep(3, i + 1, t, { state: i < 3 ? 'watched' : 'downloading', progress: 42, runtime: 34 })) },
      { n: 4, year: 2025, eps: Array.from({ length: 10 }, (_, i) => ep(4, i + 1, null, { state: 'missing', runtime: 34 })) },
    ],
    next: { s: 3, n: 4 },
  });

  add('shogun', {
    type: 'show', title: 'Shōgun', year: 2024, years: '2024–', network: 'FX', rating: 8.5, runtime: 60,
    genres: ['Drama', 'History', 'War'], creator: 'Rachel Kondo & Justin Marks', status: 'Returning series',
    overview: 'In feudal Japan, Lord Yoshii Toranaga fights for his life as his enemies on the Council of Regents unite against him, and a shipwrecked English sailor becomes his unlikely piece in the game.',
    cast: [['Hiroyuki Sanada', 'Yoshii Toranaga'], ['Cosmo Jarvis', 'John Blackthorne'], ['Anna Sawai', 'Toda Mariko'], ['Tadanobu Asano', 'Kashigi Yabushige']],
    art: { motif: 'sun', palette: ['#16100D', '#2A1B14', '#C8322B', '#D9D2C5', '#B8873E'], font: 'serif', ink: '#EFE6D8', posterTitle: 'Shōgun' },
    trakt: { rating: 9, progress: '6 of 10 watched', last: 'Watched E06 last week' },
    seasons: [{ n: 1, year: 2024, eps: ['Anjin', 'Servants of Two Masters', 'Tomorrow Is Tomorrow', 'The Eightfold Fence', 'Broken to the Fist', 'Ladies of the Willow World', 'A Stick of Time', 'The Abyss of Life', 'Crimson Sky', 'A Dream of a Dream'].map((t, i) => ep(1, i + 1, t, { state: i < 6 ? 'watched' : 'ready', runtime: [70, 58, 57, 54, 57, 63, 57, 58, 53, 66][i] })) }],
    next: { s: 1, n: 7 },
  });

  const dwSpecials = ['Children in Need Special', 'The Christmas Invasion', 'Attack of the Graske', 'Tardisodes', 'The Runaway Bride', 'Voyage of the Damned', 'Time Crash', 'The Next Doctor', 'Planet of the Dead', 'The Waters of Mars', 'The End of Time, Part One', 'The End of Time, Part Two'];
  const dwS3 = ['Smith and Jones', 'The Shakespeare Code', 'Gridlock', 'Daleks in Manhattan', 'Evolution of the Daleks', 'The Lazarus Experiment', '42', 'Human Nature', 'The Family of Blood', 'Blink', 'Utopia', 'The Sound of Drums', 'Last of the Time Lords'];
  add('doctor-who', {
    type: 'show', title: 'Doctor Who', year: 2005, years: '2005–2022', network: 'BBC One', rating: 7.5, runtime: 45,
    genres: ['Sci-Fi', 'Adventure', 'Drama'], creator: 'Russell T Davies', status: 'Ended',
    overview: 'The Doctor, a time-travelling alien, explores the universe in the TARDIS with a rotating cast of companions, saving civilisations and righting wrongs.',
    cast: [['David Tennant', 'The Doctor'], ['Billie Piper', 'Rose Tyler'], ['Freema Agyeman', 'Martha Jones'], ['Catherine Tate', 'Donna Noble'], ['John Barrowman', 'Captain Jack Harkness']],
    art: { motif: 'box', palette: ['#0B1B44', '#16357F', '#1E4FA6', '#9CC3F0', '#FFF4C8'], font: 'cond', ink: '#FFFFFF' },
    trakt: { rating: 8, progress: '26 of 63 watched', last: 'Watched S02E13 “Doomsday” on Monday' },
    review: { count: 3, job: 'doctor-who' },
    seasons: [
      { n: 0, name: 'Specials', eps: dwSpecials.map((t, i) => ep(0, i + 1, t, { state: i < 2 ? 'watched' : i === 4 || i === 5 || i === 6 ? 'review' : i < 4 ? 'ready' : 'ready', runtime: [7, 60, 4, 3, 64, 71, 8, 60, 60, 62, 60, 75][i] })) },
      { n: 1, year: 2005, eps: Array.from({ length: 13 }, (_, i) => ep(1, i + 1, ['Rose', 'The End of the World', 'The Unquiet Dead', 'Aliens of London', 'World War Three', 'Dalek', 'The Long Game', "Father's Day", 'The Empty Child', 'The Doctor Dances', 'Boom Town', 'Bad Wolf', 'The Parting of the Ways'][i], { state: 'watched' })) },
      { n: 2, year: 2006, eps: Array.from({ length: 13 }, (_, i) => ep(2, i + 1, ['New Earth', 'Tooth and Claw', 'School Reunion', 'The Girl in the Fireplace', 'Rise of the Cybermen', 'The Age of Steel', 'The Idiot’s Lantern', 'The Impossible Planet', 'The Satan Pit', 'Love & Monsters', 'Fear Her', 'Army of Ghosts', 'Doomsday'][i], { state: 'watched' })) },
      { n: 3, year: 2007, eps: dwS3.map((t, i) => ep(3, i + 1, t, { state: 'ready' })) },
      { n: 4, year: 2008, eps: Array.from({ length: 13 }, (_, i) => ep(4, i + 1, ['Partners in Crime', 'The Fires of Pompeii', 'Planet of the Ood', 'The Sontaran Stratagem', 'The Poison Sky', "The Doctor's Daughter", 'The Unicorn and the Wasp', 'Silence in the Library', 'Forest of the Dead', 'Midnight', 'Turn Left', 'The Stolen Earth', "Journey's End"][i], { state: 'missing' })) },
    ],
    next: { s: 0, n: 5 },
  });

  add('bluey', {
    type: 'show', title: 'Bluey', year: 2018, years: '2018–', network: 'ABC Kids', rating: 8.7, runtime: 7,
    genres: ['Animation', 'Family', 'Kids'], creator: 'Joe Brumm', status: 'Returning series',
    overview: 'Bluey, a boundless Blue Heeler pup, turns everyday family life in Brisbane into extraordinary games with her little sister Bingo and their endlessly patient parents.',
    cast: [['Dave McCormack', 'Bandit'], ['Melanie Zanetti', 'Chilli']],
    art: { motif: 'blobs', palette: ['#9BD8F5', '#6FC1EC', '#4C7FD6', '#F5A04A', '#FFE07A'], font: 'round', ink: '#173A7A', titleTop: true },
    trakt: { progress: '61 of 153 watched' },
    seasons: [
      { n: 1, year: 2018, eps: Array.from({ length: 52 }, (_, i) => ep(1, i + 1, null, { state: 'watched', runtime: 7 })) },
      { n: 2, year: 2020, eps: Array.from({ length: 52 }, (_, i) => ep(2, i + 1, null, { state: i < 9 ? 'watched' : 'missing', runtime: 7 })) },
      { n: 3, year: 2021, eps: Array.from({ length: 49 }, (_, i) => ep(3, i + 1, null, { state: 'ready', runtime: 7 })) },
    ],
  });

  add('pluribus', {
    type: 'show', title: 'Pluribus', year: 2025, years: '2025–', network: 'Apple TV+', rating: 8.2, runtime: 55,
    genres: ['Sci-Fi', 'Drama'], creator: 'Vince Gilligan', status: 'Returning series',
    overview: 'The most miserable person on Earth must save the world from happiness.',
    cast: [['Rhea Seehorn', 'Carol Sturka'], ['Karolina Wydra', 'Zosia'], ['Carlos-Manuel Vesga', 'Manousos']],
    art: { motif: 'crowd', palette: ['#F6C843', '#E9A92B', '#FFF1BF', '#2A1C16', '#B26A12'], font: 'sans', ink: '#2A1C16', titleTop: true },
    trakt: { progress: '2 of 9 watched', last: 'Watched E02 elsewhere' },
    notInLibrary: true,
    seasons: [{ n: 1, year: 2025, eps: Array.from({ length: 9 }, (_, i) => ep(1, i + 1, null, { state: i < 2 ? 'watched' : 'missing', runtime: 55 })) }],
    next: { s: 1, n: 3 },
  });

  // ------------------------------------------------------------------ movies
  add('dune-part-two', {
    type: 'movie', title: 'Dune: Part Two', year: 2024, runtime: 166, rating: 8.1, genres: ['Sci-Fi', 'Adventure'], director: 'Denis Villeneuve',
    overview: 'Paul Atreides unites with Chani and the Fremen on a path of revenge against the conspirators who destroyed his family, while trying to prevent a terrible future only he can foresee.',
    cast: [['Timothée Chalamet', 'Paul Atreides'], ['Zendaya', 'Chani'], ['Rebecca Ferguson', 'Lady Jessica'], ['Javier Bardem', 'Stilgar'], ['Austin Butler', 'Feyd-Rautha'], ['Florence Pugh', 'Princess Irulan']],
    trailer: '3:02',
    art: { motif: 'dunes', palette: ['#F5B76A', '#C7642F', '#B35A2A', '#8E4220', '#FFE3B0'], font: 'cond', ink: '#2A140A', posterTitle: 'Dune Part Two' },
    trakt: { watched: 'Mar 9, 2024', rating: 9 },
    lib: 'ready',
    torrent: { name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', size: '18.4 GB', q: ['2160p', 'WEB-DL', 'DV', 'Atmos'], health: 'Healthy · 640 seeders' },
    reason: 'You rated Dune 9/10 and rewatch Villeneuve.',
    subs: ['English', 'Azerbaijani'],
  });
  add('interstellar', {
    type: 'movie', title: 'Interstellar', year: 2014, runtime: 169, rating: 8.4, genres: ['Sci-Fi', 'Drama', 'Adventure'], director: 'Christopher Nolan',
    overview: 'With Earth failing, a former pilot leaves his children behind to lead a mission through a wormhole in search of a new home for humanity.',
    cast: [['Matthew McConaughey', 'Cooper'], ['Anne Hathaway', 'Brand'], ['Jessica Chastain', 'Murph'], ['Michael Caine', 'Professor Brand'], ['Mackenzie Foy', 'Young Murph']],
    trailer: '2:49',
    art: { motif: 'ring', palette: ['#07090D', '#000000', '#3B2A14', '#F2A541', '#FFE3A3'], font: 'sans', ink: '#E8E2D6' },
    trakt: { watched: 'Nov 2014', rating: 10 },
    lib: 'none',
    reason: 'Your highest-rated film. The IMAX rerelease is out this month.',
  });
  add('perfect-days', {
    type: 'movie', title: 'Perfect Days', year: 2023, runtime: 124, rating: 7.8, genres: ['Drama'], director: 'Wim Wenders',
    overview: 'Hirayama cleans public toilets in Tokyo and seems content with his structured everyday life: music, books, and the trees he photographs.',
    cast: [['Kōji Yakusho', 'Hirayama'], ['Tokio Emoto', 'Takashi'], ['Arisa Nakano', 'Niko'], ['Aoi Yamada', 'Aya']],
    art: { motif: 'canopy', palette: ['#3D6B45', '#1F3A28', '#5E8F55', '#86B36A', '#F7F1C2'], font: 'serif', ink: '#F7F1E3' },
    lib: 'downloading', dl: 63,
    torrent: { name: 'Perfect.Days.2023.1080p.MUBI.WEB-DL.DDP5.1.H.264-FLUX', size: '6.2 GB', q: ['1080p', 'WEB-DL', 'DDP5.1'], health: 'Healthy · 88 seeders' },
    reason: 'Quiet, kind and slow, like Past Lives, which you rated 9.',
  });
  add('past-lives', {
    type: 'movie', title: 'Past Lives', year: 2023, runtime: 106, rating: 7.6, genres: ['Drama', 'Romance'], director: 'Celine Song',
    overview: 'Nora and Hae Sung, two deeply connected childhood friends, are wrested apart, then reunited twenty years later in New York for one fateful week.',
    cast: [['Greta Lee', 'Nora'], ['Teo Yoo', 'Hae Sung'], ['John Magaro', 'Arthur']],
    art: { motif: 'two', palette: ['#F2B39A', '#6C6FA3', '#2B2440', '#3A3156', '#FFE2A8'], font: 'serif', ink: '#FFF6EC', titleTop: true },
    trakt: { watched: 'Jan 2024', rating: 9 },
    lib: 'added',
  });

  // ------------------------------------------------------------------ other titles (shelves, search, discover)
  const extra = (id, type, title, year, motif, palette, o = {}) => add(id, { type, title, year, genres: o.genres || [], rating: o.rating || 7.8, art: { motif, palette, font: o.font || 'sans', ink: o.ink || '#fff', titleTop: o.top }, ...o });
  extra('silo', 'show', 'Silo', 2023, 'spiral', ['#3B4A3F', '#1C231E', '#6F7F62', '#A3B08A', '#E8D9A8'], { genres: ['Sci-Fi', 'Drama'], rating: 8.1, network: 'Apple TV+' });
  extra('dark', 'show', 'Dark', 2017, 'moon', ['#0E1A22', '#05090C', '#1E2F38', '#3A5260', '#E6E1C5'], { genres: ['Sci-Fi', 'Mystery'], rating: 8.4, font: 'serif', original_language: 'de' });
  extra('mr-robot', 'show', 'Mr. Robot', 2015, 'halftone', ['#161616', '#050505', '#C62828', '#ffffff', '#ffffff'], { genres: ['Drama', 'Crime'], rating: 8.3, font: 'mono' });
  extra('the-leftovers', 'show', 'The Leftovers', 2014, 'mountains', ['#B9C6CF', '#E9E3D5', '#7D8C94', '#5C6970', '#F4E7B5'], { genres: ['Drama', 'Mystery'], rating: 7.8, font: 'serif', ink: '#2F3A40' });
  extra('station-eleven', 'show', 'Station Eleven', 2021, 'arch', ['#1B2E3C', '#3E5C6E', '#E2B65B', '#C04A2F', '#F7E7B4'], { genres: ['Drama', 'Sci-Fi'], rating: 7.6, font: 'serif' });
  extra('maniac', 'show', 'Maniac', 2018, 'grid', ['#F3D0D8', '#B9D4F0', '#E86A92', '#4FA3D9', '#FFE07A'], { genres: ['Comedy', 'Sci-Fi'], rating: 7.5, ink: '#2A1C16' });
  extra('devs', 'show', 'Devs', 2020, 'waves', ['#2B2208', '#0E0A02', '#C9A33A', '#8E6F1E', '#F4D97B'], { genres: ['Sci-Fi', 'Thriller'], rating: 7.6 });
  extra('slow-horses', 'show', 'Slow Horses', 2022, 'city', ['#2F3A33', '#141A16', '#4F5B50', '#D6A84B', '#F2D27A'], { genres: ['Thriller', 'Drama'], rating: 8.0, network: 'Apple TV+', font: 'cond' });
  extra('andor', 'show', 'Andor', 2022, 'split', ['#2A2D33', '#B5492E', '#E7DED0', '#1A1C20', '#F4E2C2'], { genres: ['Sci-Fi', 'Drama'], rating: 8.3, font: 'cond' });
  extra('only-murders', 'show', 'Only Murders in the Building', 2021, 'city', ['#2C2140', '#120C1C', '#4B3A68', '#F2B84B', '#FFE7A8'], { genres: ['Comedy', 'Mystery'], rating: 8.0, font: 'serif' });
  extra('pachinko', 'show', 'Pachinko', 2022, 'waves', ['#E7D3B8', '#C99C6B', '#8C5A3C', '#5E3A2A', '#F6E7CE'], { genres: ['Drama'], rating: 8.1, font: 'serif', ink: '#3A2418' });
  extra('murderbot', 'show', 'Murderbot', 2025, 'grid', ['#C9D7DF', '#7F98A8', '#2F4858', '#E36C3A', '#F2F5F7'], { genres: ['Sci-Fi', 'Comedy'], rating: 7.9, network: 'Apple TV+', ink: '#13232E', top: true });
  extra('the-good-place', 'show', 'The Good Place', 2016, 'blobs', ['#BDE3D3', '#8DCDB6', '#F4F1E8', '#F28C6B', '#FFD86B'], { genres: ['Comedy', 'Fantasy'], rating: 8.0, font: 'round', ink: '#1F4A3D', top: true });
  extra('russian-doll', 'show', 'Russian Doll', 2019, 'split', ['#1C1C1C', '#C7302B', '#F2E6D8', '#E06D3F', '#FFD7B0'], { genres: ['Comedy', 'Mystery'], rating: 7.7, font: 'serif' });
  extra('upload', 'show', 'Upload', 2020, 'city', ['#B9E0F2', '#7FB4D6', '#E7F4FA', '#F6A6B2', '#FFFFFF'], { genres: ['Comedy', 'Sci-Fi'], rating: 7.8, ink: '#1B3A52', top: true });
  extra('mythic-quest', 'show', 'Mythic Quest', 2020, 'grid', ['#5B2C83', '#2A1240', '#F2C230', '#E85D75', '#7FD1C7'], { genres: ['Comedy'], rating: 7.6, font: 'cond' });
  extra('delicious-in-dungeon', 'show', 'Delicious in Dungeon', 2024, 'halftone', ['#F6D9A8', '#E8A85C', '#B5562E', '#7A3418', '#FFF2D6'], { genres: ['Animation', 'Fantasy'], rating: 8.5, anime: true, font: 'round', ink: '#5A230E' });
  extra('apothecary-diaries', 'show', 'The Apothecary Diaries', 2023, 'arch', ['#7E1F2B', '#3E0E14', '#E8C99B', '#C7425B', '#FFE9C9'], { genres: ['Animation', 'Mystery'], rating: 8.6, anime: true, font: 'serif' });
  extra('mushishi', 'show', 'Mushishi', 2005, 'canopy', ['#5C7B5F', '#2C3D2E', '#86A57E', '#B5C99A', '#F4F1D0'], { genres: ['Animation', 'Fantasy'], rating: 8.6, anime: true, font: 'serif' });
  extra('vinland-saga', 'show', 'Vinland Saga', 2019, 'waves', ['#AFC4D0', '#5E7889', '#E8EEF1', '#324B5C', '#F7F4EE'], { genres: ['Animation', 'Action'], rating: 8.7, anime: true, font: 'cond', ink: '#1E3442' });
  extra('odd-taxi', 'show', 'Odd Taxi', 2021, 'city', ['#F2C14E', '#E07A3F', '#2C3E50', '#F7E7B4', '#FFFFFF'], { genres: ['Animation', 'Mystery'], rating: 8.3, anime: true, font: 'round', ink: '#2C3E50' });
  extra('violet-evergarden', 'show', 'Violet Evergarden', 2018, 'moon', ['#9CB8D9', '#E9EEF6', '#7C98BE', '#C9D7EA', '#FFF3D6'], { genres: ['Animation', 'Drama'], rating: 8.6, anime: true, font: 'serif', ink: '#2E3F5C' });
  extra('aftersun', 'movie', 'Aftersun', 2022, 'waves', ['#7FC4C9', '#2F7F8A', '#F3E2C7', '#E78E6E', '#FFF6E8'], { genres: ['Drama'], rating: 7.7, runtime: 101, font: 'serif', top: true });
  extra('columbus', 'movie', 'Columbus', 2017, 'arch', ['#F1EFE9', '#DAD6CC', '#9FA7A3', '#5B6F6A', '#F4C77A'], { genres: ['Drama'], rating: 7.2, runtime: 104, font: 'sans', ink: '#2F3A38' });
  extra('paterson', 'movie', 'Paterson', 2016, 'split', ['#C9D3D6', '#6E8B92', '#F2EDE3', '#304A52', '#F5D38A'], { genres: ['Drama', 'Comedy'], rating: 7.3, runtime: 118, font: 'serif', ink: '#1E3036' });
  extra('drive-my-car', 'movie', 'Drive My Car', 2021, 'mountains', ['#9EB7C7', '#E8E4DA', '#C0392B', '#6F8796', '#F7F2E6'], { genres: ['Drama'], rating: 7.5, runtime: 179, font: 'sans', ink: '#2B2B2B' });
  extra('leave-no-trace', 'movie', 'Leave No Trace', 2018, 'canopy', ['#4C6B4F', '#1E2E22', '#6E8F65', '#94B07E', '#EDE7C4'], { genres: ['Drama'], rating: 7.2, runtime: 109, font: 'serif' });
  extra('dune-part-three', 'movie', 'Dune: Part Three', 2026, 'dunes', ['#E9C3A0', '#8B5A62', '#7A4652', '#5A2F3E', '#FFE9D2'], { genres: ['Sci-Fi', 'Adventure'], rating: 0, runtime: 0, font: 'cond', ink: '#2A140A', posterTitle: 'Dune Part Three', comingSoon: 'Dec 18' });
  extra('the-odyssey', 'movie', 'The Odyssey', 2026, 'waves', ['#1D3557', '#0B1526', '#457B9D', '#A8DADC', '#F1FAEE'], { genres: ['Adventure', 'Fantasy'], rating: 7.9, runtime: 0, font: 'serif' });
  extra('the-diplomat', 'show', 'The Diplomat', 2023, 'split', ['#1F2A44', '#8C1C13', '#F2E9DC', '#C9A227', '#FFFFFF'], { genres: ['Drama', 'Thriller'], rating: 7.9, font: 'serif' });
  extra('the-pitt', 'show', 'The Pitt', 2025, 'grid', ['#D8E6EC', '#9DB9C6', '#1F5C7A', '#E04F4F', '#FFFFFF'], { genres: ['Drama'], rating: 8.6, font: 'cond', ink: '#123A4C', top: true });
  extra('arrival', 'movie', 'Arrival', 2016, 'moon', ['#B7C0C7', '#5C666D', '#3E474D', '#2A3035', '#F1F3F4'], { genres: ['Sci-Fi', 'Drama'], rating: 7.6, runtime: 116, font: 'sans', ink: '#1F2529' });
  extra('heat', 'movie', 'Heat', 1995, 'city', ['#1E3A5F', '#0A1626', '#2E4A6B', '#E8A33D', '#F8D58C'], { genres: ['Crime', 'Thriller'], rating: 7.9, runtime: 170, font: 'cond' });
  extra('prisoners', 'movie', 'Prisoners', 2013, 'mountains', ['#59636B', '#2C3237', '#3F474D', '#20262A', '#D9DEE1'], { genres: ['Thriller', 'Crime'], rating: 8.1, runtime: 153, font: 'serif' });

  // reasons for AI shelves (one line each, grounded in the history)
  const R = {
    silo: 'Another sealed world with a company that lies about it.', dark: 'Puzzle-box mystery with the same slow, cold dread.',
    'mr-robot': 'Corporate paranoia and an unreliable narrator.', 'the-leftovers': 'Big unanswered questions, handled with real feeling.',
    'station-eleven': 'Melancholy and hopeful, like Severance’s best episodes.', maniac: 'Lighter and stranger, from the same brain-hacking corner.',
    devs: 'A beautiful, eerie tech campus with secrets.', 'slow-horses': 'New season · you finished the last one in 4 days.',
    andor: 'Completed series · you liked its Lumon-grade bureaucracy.', 'only-murders': 'New season · cosy mystery for weeknights.',
    pachinko: 'New season · generational, like Past Lives at scale.', murderbot: 'New season · corporate sci-fi, but funny.',
    'the-pitt': 'Your friends rate it highly; one shift per episode.',
    'delicious-in-dungeon': 'Frieren’s gentle wonder, with cooking instead of grief.', 'apothecary-diaries': 'Clever lead, palace mysteries, 9.0 from people like you.',
    mushishi: 'Episodic and meditative, the closest thing to Frieren’s quiet.', 'vinland-saga': 'Season 2 is the farm arc: slower, kinder.',
    'odd-taxi': 'A twisty little mystery; 13 episodes.', 'violet-evergarden': 'Grief and growing up, like Frieren.',
    aftersun: 'Short and devastating, if Past Lives hit you.', columbus: 'Architecture, long takes and quiet talk.',
    paterson: 'A gentle routine film, Perfect Days’ cousin.', 'drive-my-car': 'Long, patient and rewarding; you finish long films.',
    'leave-no-trace': 'Small, warm and honest; 109 minutes.',
    'dune-part-three': 'You rated Part Two 9/10.', 'the-odyssey': 'Nolan · you’ve rated four of his films 9 or higher.',
    interstellar: 'Your highest-rated film is back in IMAX.', 'perfect-days': 'Quiet, kind and slow, like Past Lives.',
    'the-good-place': 'Workplace afterlife mystery with jokes. Twisty like Severance, half the dread.',
    'russian-doll': 'Loops and puzzles, but funny. 8 short episodes.',
    upload: 'A corporate afterlife satire, bright and silly.', 'mythic-quest': 'Office comedy with heart; no dread at all.',
    'the-diplomat': 'New season · smart and brisk.', arrival: 'Thoughtful sci-fi, under 2 hours.',
  };
  Object.entries(R).forEach(([k, v]) => { if (T[k]) T[k].reason = T[k].reason || v; });

  const shelves = [
    { slug: 'because-severance', title: 'Because you loved Severance', sub: 'Slow-burn mysteries with a sting', items: ['silo', 'dark', 'mr-robot', 'the-leftovers', 'station-eleven', 'maniac', 'devs'] },
    { slug: 'new-this-month', title: 'New this month for you', sub: 'Fresh seasons of things you finish', items: ['slow-horses', 'the-diplomat', 'only-murders', 'pachinko', 'murderbot', 'the-pitt', 'andor'] },
    { slug: 'anime', title: 'Anime you’ll like', sub: 'Because Frieren is your 10/10', items: ['delicious-in-dungeon', 'apothecary-diaries', 'mushishi', 'vinland-saga', 'odd-taxi', 'violet-evergarden'] },
    { slug: 'hidden-gems', title: 'Hidden gems', sub: 'Quiet films people like you rate highly', items: ['aftersun', 'columbus', 'paterson', 'drive-my-car', 'leave-no-trace', 'perfect-days'] },
    { slug: 'coming-soon', title: 'Coming soon', sub: 'On your radar', items: ['dune-part-three', 'the-odyssey', 'interstellar'] },
  ];

  // Up next (from Trakt progress), each with availability
  const upNext = [
    { show: 'frieren', s: 1, n: 29, label: 'S01E29', title: 'Episode 29', av: 'needs_torrent', note: 'Released as “S2 - 01”' },
    { show: 'severance', s: 2, n: 6, label: 'S02E06', title: 'Attila', av: 'ready', progress: 0 },
    { show: 'the-bear', s: 3, n: 4, label: 'S03E04', title: 'Violet', av: 'downloading', pct: 42 },
    { show: 'shogun', s: 1, n: 7, label: 'S01E07', title: 'A Stick of Time', av: 'ready', progress: 18 },
    { show: 'doctor-who', s: 0, n: 5, label: 'Special', title: 'The Runaway Bride', av: 'review' },
    { show: 'pluribus', s: 1, n: 3, label: 'S01E03', title: 'Episode 3', av: 'not_in_library' },
    { show: 'one-piece', abs: 1160, label: 'Ep 1160', title: 'Episode 1160', av: 'airs', airs: 'Airs Sun' },
  ];

  const newEpisodes = [
    { show: 'one-piece', label: 'Ep 1159', title: 'Episode 1159', when: 'Sunday', av: 'ready' },
    { show: 'the-bear', label: 'S04', title: '10 episodes', when: 'Season 4 is out', av: 'needs_torrent' },
    { show: 'frieren', label: 'S01E29–38', title: '10 episodes', when: 'Second cour finished in March', av: 'needs_torrent' },
  ];

  const watchlist = ['aftersun', 'andor', 'pachinko', 'drive-my-car', 'mushishi', 'the-leftovers', 'interstellar', 'arrival'];
  const trending = ['severance', 'the-pitt', 'pluribus', 'one-piece', 'dune-part-two', 'slow-horses', 'murderbot', 'andor', 'the-bear', 'shogun'];

  // ------------------------------------------------------------------ torrent search results
  const rel = (raw, o) => ({ raw, ...o });
  const releases = {
    'severance-s02e05': {
      ctx: { title: 'severance', label: 'Severance S02E05', scope: 'episode', query: 'Severance S02E05' },
      total: 143,
      items: [
        rel('Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', { emph: 'Severance.S02E05', size: '4.1 GB', se: 312, le: 18, age: '8 mo', idx: 'TorrentLeech', tags: [['2160p', 'hi'], ['WEB-DL'], ['x265'], ['DV', 'hi'], ['HDR'], ['Atmos', 'hi'], ['FLUX', 'group']], best: true,
          why: ['2160p WEB-DL', 'Dolby Vision', '4.1 GB', '312 seeders', 'matches your quality profile'] }),
        rel('Severance.S02E05.1080p.ATVP.WEB-DL.DDP5.1.H.264-FLUX', { emph: 'Severance.S02E05', size: '2.3 GB', se: 540, le: 22, age: '8 mo', idx: 'TorrentLeech', tags: [['1080p'], ['WEB-DL'], ['x264'], ['DDP5.1'], ['FLUX', 'group']] }),
        rel('Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', { emph: 'Severance.S02', size: '41.3 GB', se: 210, le: 31, age: '7 mo', idx: 'TorrentLeech', tags: [['Season 2 pack', 'pack'], ['2160p', 'hi'], ['WEB-DL'], ['x265'], ['DV', 'hi'], ['Atmos', 'hi'], ['FLUX', 'group']] }),
        rel('Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.HDR.H.265-NTb', { emph: 'Severance.S02E05', size: '4.0 GB', se: 128, le: 9, age: '8 mo', idx: 'IPTorrents', tags: [['2160p', 'hi'], ['WEB-DL'], ['x265'], ['HDR'], ['NTb', 'group']] }),
        rel('Severance.S02E05.720p.WEB.h264-ETHEL', { emph: 'Severance.S02E05', size: '910 MB', se: 88, le: 4, age: '8 mo', idx: 'EZTV', tags: [['720p'], ['WEB'], ['x264'], ['ETHEL', 'group']] }),
        rel('Severance.S02E05.1080p.WEBRip.x265-KONTRAST', { emph: 'Severance.S02E05', size: '812 MB', se: 61, le: 7, age: '8 mo', idx: 'The Pirate Bay', tags: [['1080p'], ['WEBRip'], ['x265'], ['KONTRAST', 'group']] }),
        rel('Severance S02E05 2160p HDR', { emph: 'Severance S02E05', size: '214 MB', se: 3, le: 40, age: '2 mo', idx: 'The Pirate Bay', tags: [['2160p'], ['Suspicious size', 'warn']], warn: ['Suspicious size: 214 MB is far too small for 52 min of 2160p.'] }),
        rel('Severance.S02E05.1080p.ATVP.WEB-DL.DDP5.1.H.264-FLUX', { emph: 'Severance.S02E05', size: '2.3 GB', se: 0, le: 0, age: '8 mo', idx: '1337x', tags: [['1080p'], ['WEB-DL'], ['x264'], ['No seeders', 'warn']], warn: ['No seeders right now. It may never finish.'] }),
      ],
      facets: { Resolution: [['2160p', 38], ['1080p', 71], ['720p', 29], ['480p', 5]], Source: [['WEB-DL', 96], ['WEBRip', 31], ['WEB', 12], ['HDTV', 4]], Codec: [['x265', 61], ['x264', 82]], 'HDR / DV': [['DV', 22], ['HDR10', 34], ['HDR10+', 6]], Audio: [['Atmos', 27], ['DDP5.1', 88], ['AAC', 41]], Pack: [['Episode', 112], ['Season pack', 26], ['Complete', 5]], Indexer: [['TorrentLeech', 52], ['IPTorrents', 39], ['EZTV', 28], ['The Pirate Bay', 18], ['1337x', 6]], Group: [['FLUX', 31], ['NTb', 18], ['ETHEL', 12], ['KONTRAST', 9]] },
    },
    'frieren-s01e29': {
      ctx: { title: 'frieren', label: 'Frieren S01E29', scope: 'episode', query: 'Frieren S01E29', also: 'Also searched: “Sousou no Frieren S2 01”, “Frieren 29”' },
      total: 22,
      items: [
        rel('[SubsPlease] Sousou no Frieren S2 (01-10) (1080p) [Batch]', { emph: 'Sousou no Frieren S2 (01-10)', size: '14.2 GB', se: 1204, le: 37, age: '7 mo', idx: 'Nyaa', tags: [['Season 2 pack · E29–E38', 'pack'], ['1080p'], ['WEB'], ['x264'], ['SubsPlease', 'group']], best: true,
          why: ['Covers E29 and the 9 missing after it', '1080p WEB', '14.2 GB', '1,204 seeders', 'your preferred group'] }),
        rel('[SubsPlease] Sousou no Frieren S2 - 01 (1080p) [8F2A6C1D].mkv', { emph: 'Sousou no Frieren S2 - 01', size: '1.4 GB', se: 412, le: 6, age: '9 mo', idx: 'Nyaa', tags: [['1080p'], ['WEB'], ['x264'], ['SubsPlease', 'group']], note: '“S2 - 01” is TMDB S01E29' }),
        rel('Frieren.Beyond.Journeys.End.S02.1080p.CR.WEB-DL.DUAL.AAC2.0.H.264-VARYG', { emph: 'Frieren.Beyond.Journeys.End.S02', size: '13.6 GB', se: 388, le: 21, age: '7 mo', idx: 'TorrentLeech', tags: [['Season 2 pack · E29–E38', 'pack'], ['1080p'], ['WEB-DL'], ['Dual audio'], ['VARYG', 'group']] }),
        rel('[ASW] Sousou no Frieren S2 - 01 [1080p HEVC x265 10Bit][AAC]', { emph: 'Sousou no Frieren S2 - 01', size: '420 MB', se: 150, le: 3, age: '9 mo', idx: 'Nyaa', tags: [['1080p'], ['x265'], ['10-bit'], ['ASW', 'group']] }),
        rel('Frieren.Beyond.Journeys.End.S02E01.2160p.CR.WEB-DL.AAC2.0.H.265-ToonsHub', { emph: 'Frieren.Beyond.Journeys.End.S02E01', size: '2.9 GB', se: 74, le: 5, age: '9 mo', idx: 'TorrentLeech', tags: [['2160p', 'hi'], ['WEB-DL'], ['x265'], ['ToonsHub', 'group']] }),
        rel('[Erai-raws] Sousou no Frieren 2nd Season - 01 [1080p][Multiple Subtitle][F3E2A1B9].mkv', { emph: 'Sousou no Frieren 2nd Season - 01', size: '1.4 GB', se: 96, le: 2, age: '9 mo', idx: 'Nyaa', tags: [['1080p'], ['WEB'], ['Multi-sub'], ['Erai-raws', 'group']] }),
        rel('Frieren Beyond Journeys End S2 E01 1080p [FREE DOWNLOAD]', { emph: 'Frieren Beyond Journeys End S2 E01', size: '3.2 MB', se: 0, le: 12, age: '3 wk', idx: 'The Pirate Bay', tags: [['Suspicious size', 'warn'], ['No seeders', 'warn']], warn: ['3.2 MB cannot be a 24-minute episode.', 'No seeders right now.'] }),
      ],
      facets: { Resolution: [['1080p', 16], ['2160p', 3], ['720p', 3]], Source: [['WEB', 12], ['WEB-DL', 8], ['BluRay', 0]], Codec: [['x264', 13], ['x265', 9]], Pack: [['Episode', 15], ['Season pack', 6], ['Complete', 1]], Indexer: [['Nyaa', 13], ['TorrentLeech', 6], ['The Pirate Bay', 3]], Group: [['SubsPlease', 5], ['VARYG', 3], ['Erai-raws', 4], ['ASW', 3]] },
    },
    'dune-part-two': {
      ctx: { title: 'dune-part-two', label: 'Dune: Part Two (2024)', scope: 'movie', query: 'Dune Part Two 2024' },
      total: 87,
      items: [
        rel('Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', { emph: 'Dune.Part.Two.2024', size: '18.4 GB', se: 640, le: 52, age: '2 y', idx: 'TorrentLeech', tags: [['2160p', 'hi'], ['WEB-DL'], ['x265'], ['DV', 'hi'], ['Atmos', 'hi'], ['FLUX', 'group']], best: true, why: ['2160p WEB-DL', 'Dolby Vision', '18.4 GB (6.6 GB/h)', '640 seeders', 'matches your quality profile'] }),
        rel('Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.Atmos.7.1-FGT', { emph: 'Dune.Part.Two.2024', size: '78.2 GB', se: 210, le: 33, age: '2 y', idx: 'IPTorrents', tags: [['2160p', 'hi'], ['REMUX'], ['DV', 'hi'], ['TrueHD Atmos', 'hi'], ['Over size limit', 'warn']], warn: ['78.2 GB is 28 GB/h; your limit is 12 GB/h.'] }),
        rel('Dune.Part.Two.2024.1080p.BluRay.DDP5.1.x264-NTb', { emph: 'Dune.Part.Two.2024', size: '14.9 GB', se: 402, le: 18, age: '2 y', idx: 'TorrentLeech', tags: [['1080p'], ['BluRay'], ['x264'], ['NTb', 'group']] }),
        rel('Dune Part Two 2024 HDCAM x264', { emph: 'Dune Part Two 2024', size: '1.6 GB', se: 12, le: 3, age: '2 y', idx: 'The Pirate Bay', tags: [['CAM', 'err'], ['x264']], warn: ['CAM: filmed in a cinema. Poor picture and sound.'] }),
      ],
      facets: { Resolution: [['2160p', 31], ['1080p', 44], ['720p', 12]], Source: [['WEB-DL', 29], ['BluRay', 38], ['REMUX', 9], ['CAM', 3]], Codec: [['x265', 40], ['x264', 47]] },
    },
  };

  // ------------------------------------------------------------------ mapping jobs
  const fr = (n, v) => `[SubsPlease] Sousou no Frieren S2 - ${String(n).padStart(2, '0')}${v ? 'v2' : ''} (1080p) [${['8F2A6C1D', '0B7E33A2', 'C51D9F04', '7A2E61B8', '1E9C04D7', 'E4B7A930', '5D03C2F1', 'A9F81E6C', '3C6D0B95', '96E2A7D4'][n - 1]}${v ? 'V2' : ''}].mkv`;
  const jobs = {
    frieren: {
      torrent: '[SubsPlease] Sousou no Frieren S2 (01-10) (1080p) [Batch]', show: 'frieren', size: '14.2 GB', files: 14,
      groups: {
        matched: [1, 2, 3, 4, 6, 7, 8, 9, 10].map((n) => ({ file: fr(n), size: '1.42 GB', target: { s: 1, n: 28 + n, title: `Episode ${28 + n}` }, conf: n === 1 ? 'high' : 'high', src: n === 1 ? 'ai' : 'rules',
          reason: n === 1 ? 'AI: “S2 - 01” → S01E29. TMDB keeps the second cour in season 1.' : 'Same offset as S2 - 01 (+28)' })),
        needs_review: [
          { file: fr(5), size: '1.41 GB', target: { s: 1, n: 33, title: 'Episode 33' }, conf: 'low', src: 'rules', reason: 'Two files claim S01E33. The v2 file is a fixed re-release.', conflict: true, suggest: 'ignore' },
          { file: fr(5, true), size: '1.42 GB', target: { s: 1, n: 33, title: 'Episode 33' }, conf: 'medium', src: 'ai', reason: 'AI: v2 replaces the original release of S2 - 05.', conflict: true },
        ],
        specials: [],
        extras: [
          { file: '[SubsPlease] Sousou no Frieren S2 - NCOP (1080p) [2B41E07A].mkv', size: '134 MB', kind: 'Extra', reason: 'Creditless opening' },
          { file: '[SubsPlease] Sousou no Frieren S2 - NCED (1080p) [77C0D5B3].mkv', size: '131 MB', kind: 'Extra', reason: 'Creditless ending' },
        ],
        unmatched: [
          { file: '[SubsPlease] Sousou no Frieren S2 - 10.5 Recap (1080p) [4E19A6C0].mkv', size: '1.38 GB', reason: 'A recap with no TMDB entry. Pick a special or ignore it.' },
        ],
      },
    },
    'one-piece': {
      torrent: 'One.Piece.1071-1085.1080p.WEB.x264-GROUP', show: 'one-piece', size: '21.3 GB', files: 15,
      groups: {
        matched: Array.from({ length: 13 }, (_, i) => ({ file: `One.Piece.${1071 + i}.1080p.WEB.x264-GROUP.mkv`, size: '1.42 GB', target: { s: 21, n: 79 + i, title: `Episode ${1071 + i}` }, conf: 'high', src: i === 0 ? 'ai' : 'rules', reason: i === 0 ? 'AI: absolute #1071 → S21E79 by air order' : `Absolute #${1071 + i} → S21E${79 + i}` })),
        needs_review: [
          { file: 'One.Piece.1084.1080p.WEB.x264-GROUP.mkv', size: '1.40 GB', target: { s: 21, n: 92, title: 'Episode 1084' }, conf: 'medium', src: 'rules', reason: 'TMDB marks S21E92 as a recap; check it’s not a special.' },
        ],
        specials: [], extras: [], unmatched: [{ file: 'One.Piece.1085.1080p.WEB.x264-GROUP.mkv', size: '1.43 GB', reason: '1085 is not on TMDB yet (air order ends at S21E92).' }],
      },
    },
    'doctor-who': {
      torrent: 'Doctor.Who.2005.Specials.720p.BluRay.x264-SiNNERS', show: 'doctor-who', size: '9.8 GB', files: 9,
      groups: {
        matched: [
          { file: 'Doctor.Who.2005.S00E02.The.Christmas.Invasion.720p.mkv', size: '1.1 GB', target: { s: 0, n: 2, title: 'The Christmas Invasion' }, conf: 'high', src: 'rules', reason: 'S00E02 matches the special by number and title' },
          { file: 'Doctor.Who.2005.S00E08.The.Next.Doctor.720p.mkv', size: '1.0 GB', target: { s: 0, n: 8, title: 'The Next Doctor' }, conf: 'high', src: 'rules', reason: 'Number and title agree' },
        ],
        needs_review: [
          { file: 'Doctor.Who.2005.S00E05.The.Runaway.Bride.720p.mkv', size: '1.1 GB', target: { s: 0, n: 5, title: 'The Runaway Bride' }, conf: 'medium', src: 'ai', reason: 'AI: title matches S00E05; the scene number matches too, but TMDB lists two “Runaway Bride” entries.' },
          { file: 'Doctor.Who.2005.Voyage.of.the.Damned.720p.mkv', size: '1.2 GB', target: { s: 0, n: 6, title: 'Voyage of the Damned' }, conf: 'medium', src: 'ai', reason: 'AI: no number in the name; matched by title.' },
          { file: 'Doctor.Who.2005.Time.Crash.Children.in.Need.720p.mkv', size: '140 MB', target: { s: 0, n: 7, title: 'Time Crash' }, conf: 'low', src: 'ai', reason: 'AI: “Children in Need” also fits S00E01. Picked by title.' },
        ],
        specials: [
          { file: 'Doctor.Who.2005.S00E09.Planet.of.the.Dead.720p.mkv', size: '1.1 GB', target: { s: 0, n: 9, title: 'Planet of the Dead' }, conf: 'high', src: 'rules', reason: 'Special by number' },
          { file: 'Doctor.Who.2005.S00E10.The.Waters.of.Mars.720p.mkv', size: '1.1 GB', target: { s: 0, n: 10, title: 'The Waters of Mars' }, conf: 'high', src: 'rules', reason: 'Special by number' },
        ],
        extras: [{ file: 'Doctor.Who.2005.Specials.Confidential.Featurette.720p.mkv', size: '420 MB', kind: 'Extra', reason: 'Behind-the-scenes featurette' }],
        unmatched: [],
      },
    },
  };

  // ------------------------------------------------------------------ activity
  const torrents = [
    { hash: 'b1c9e4', name: 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', title: 'severance', what: 'Season 2 · 10 episodes', state: 'streaming', pct: 84, down: '22.4 MB/s', up: '1.6 MB/s', peers: '41 peers · 33 seeds', eta: '6 min', size: '41.3 GB', reader: 'Apple TV · Infuse is playing S02E06 “Attila”' },
    { hash: '3f0a77', name: 'The.Bear.S03.1080p.HULU.WEB-DL.DDP5.1.H.264-NTb', title: 'the-bear', what: 'Season 3 · 10 episodes', state: 'downloading', pct: 42, down: '8.4 MB/s', up: '1.1 MB/s', peers: '38 peers · 31 seeds', eta: '9 min', size: '12.6 GB' },
    { hash: '9e21d0', name: 'Perfect.Days.2023.1080p.MUBI.WEB-DL.DDP5.1.H.264-FLUX', title: 'perfect-days', what: 'Movie', state: 'downloading', pct: 63, down: '5.2 MB/s', up: '0.4 MB/s', peers: '12 peers · 9 seeds', eta: '7 min', size: '6.2 GB' },
    { hash: 'c47b12', name: 'Doctor.Who.2005.Specials.720p.BluRay.x264-SiNNERS', title: 'doctor-who', what: 'Specials · 3 need review', state: 'review', pct: 100, down: '—', up: '0.2 MB/s', peers: '6 peers', size: '9.8 GB' },
    { hash: '5aa310', name: 'Shogun.2024.S01.2160p.DSNP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', title: 'shogun', what: 'Season 1 · 10 episodes', state: 'idle', pct: 100, size: '47.9 GB' },
    { hash: '72dd04', name: 'Bluey.S03.1080p.DSNP.WEB-DL.DDP5.1.H.264-NTb', title: 'bluey', what: 'Season 3 · 49 episodes', state: 'idle', pct: 100, size: '14.1 GB' },
    { hash: 'e0b6f9', name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', title: 'dune-part-two', what: 'Movie', state: 'paused', pct: 100, size: '18.4 GB' },
  ];

  const threads = [
    { id: 'lighter', q: 'something like Severance but lighter', when: 'Today' },
    { id: 'thriller', q: 'a 90-minute thriller for tonight', when: 'Yesterday' },
    { id: 'frieren-vibe', q: 'anime with a Frieren vibe', when: 'Last week' },
  ];
  const askResults = {
    lighter: ['the-good-place', 'russian-doll', 'murderbot', 'upload', 'mythic-quest', 'only-murders'],
    thriller: ['prisoners', 'heat', 'arrival'],
    'frieren-vibe': ['mushishi', 'delicious-in-dungeon', 'violet-evergarden', 'apothecary-diaries'],
  };

  const library = ['severance', 'frieren', 'the-bear', 'shogun', 'doctor-who', 'bluey', 'one-piece', 'dune-part-two', 'perfect-days', 'past-lives', 'silo', 'arrival', 'heat', 'aftersun'];
  const libState = {
    severance: { kind: 'show', ready: 17, total: 19, watched: 14, status: 'watching', dl: 1 },
    frieren: { kind: 'show', ready: 28, total: 38, watched: 28, status: 'incomplete' },
    'the-bear': { kind: 'show', ready: 21, total: 38, watched: 21, status: 'downloading', dl: 7, pct: 42 },
    shogun: { kind: 'show', ready: 10, total: 10, watched: 6, status: 'watching' },
    'doctor-who': { kind: 'show', ready: 47, total: 64, watched: 28, status: 'review', review: 3 },
    bluey: { kind: 'show', ready: 110, total: 153, watched: 61, status: 'incomplete' },
    'one-piece': { kind: 'show', ready: 1159, total: 1160, watched: 1159, status: 'watching' },
    'dune-part-two': { kind: 'movie', status: 'ready', watched: true },
    'perfect-days': { kind: 'movie', status: 'downloading', pct: 63 },
    'past-lives': { kind: 'movie', status: 'missing', watched: true },
    silo: { kind: 'show', ready: 20, total: 20, watched: 0, status: 'ready' },
    arrival: { kind: 'movie', status: 'ready' },
    heat: { kind: 'movie', status: 'ready', watched: true },
    aftersun: { kind: 'movie', status: 'missing' },
  };

  window.DATA = { T, shelves, upNext, newEpisodes, watchlist, trending, releases, jobs, torrents, threads, askResults, library, libState,
    webdav: 'https://momoshtrem.duckdns.org:36911',
    user: { trakt: 'couchcat', lastSync: '12 min ago' },
  };
})();
