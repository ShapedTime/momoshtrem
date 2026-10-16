package release

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// parseCases are real release names: the identify corpus's torrents, the
// prototype's fixtures (docs/redesign/design-cinematic/js/data.js), scene
// and fansub names. want is summary's rendering: resolution, source and
// codec, then only the attributes that are set, then the pack.
var parseCases = []struct {
	name  string
	title string
	year  int
	want  string
}{
	// The identify corpus.
	{"1923.S01.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb", "1923", 0, "1080p WEB-DL x264 audio:DD+ ch:5.1 g:NTb | season Season 1 s1"},
	{"9-1-1.S07.1080p.WEB.h264-ETHEL", "9-1-1", 0, "1080p WEB x264 g:ETHEL | season Season 7 s7"},
	{"Attack on Titan (2013) Complete Series [BD 1080p]", "Attack on Titan", 2013, "1080p BluRay unknown | complete Complete"},
	{"Bluey Season 3 (2021) [1080p]", "Bluey", 2021, "1080p unknown unknown | season Season 3 s3"},
	{"Bluey.S01.1080p.DSNP.WEB-DL.AAC2.0.H.264-GRP", "Bluey", 0, "1080p WEB-DL x264 audio:AAC ch:2.0 g:GRP | season Season 1 s1"},
	{"Breaking Bad (2008) Season 1-5 Complete 1080p BluRay x265", "Breaking Bad", 2008, "1080p BluRay x265 | multi_season Seasons 1–5 s1,2,3,4,5"},
	{"Breaking Bad S04 (1080p)", "Breaking Bad", 0, "1080p unknown unknown | season Season 4 s4"},
	{"Breaking.Bad.Complete.Series.DVDRip.XviD-GRP", "Breaking Bad", 0, "unknown DVD other g:GRP | complete Complete"},
	{"Breaking.Bad.S02.1080p.and.720p.BluRay.x264-GRP", "Breaking Bad", 0, "1080p BluRay x264 g:GRP | season Season 2 s2"},
	{"Breaking.Bad.S02E03.Bit.by.a.Dead.Bee.720p.BluRay.x264-GRP", "Breaking Bad", 0, "720p BluRay x264 g:GRP | episode S02E03 s2 e3-3"},
	{"Breaking.Bad.S03.720p.BluRay.x264-DEMAND", "Breaking Bad", 0, "720p BluRay x264 g:DEMAND | season Season 3 s3"},
	{"Chernobyl.2019.1080p.BluRay.x264-GRP", "Chernobyl", 2019, "1080p BluRay x264 g:GRP"},
	{"Chernobyl.S01.BLURAY-GRP", "Chernobyl", 0, "unknown BluRay unknown g:GRP | season Season 1 s1"},
	{"Doctor Who (2005) Series 1 DVDRip", "Doctor Who", 2005, "unknown DVD unknown | season Season 1 s1"},
	{"Doctor Who (2005) Specials", "Doctor Who", 2005, "unknown unknown unknown | season Specials s0"},
	{"Doctor.Who.2005.S04.720p.BluRay.x264-GRP", "Doctor Who", 2005, "720p BluRay x264 g:GRP | season Season 4 s4"},
	{"Doctor.Who.2005.Specials.1080p.BluRay.x264-GRP", "Doctor Who", 2005, "1080p BluRay x264 g:GRP | season Specials s0"},
	{"Extras.Complete.Series.1080p.BluRay.x264-GRP", "Extras", 0, "1080p BluRay x264 g:GRP | complete Complete"},
	{"Friends Season 2 (DVDRip)", "Friends", 0, "unknown DVD unknown | season Season 2 s2"},
	{"Frieren.Beyond.Journeys.End.S02.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG", "Frieren Beyond Journeys End", 0, "1080p WEB-DL x264 audio:AAC ch:2.0 g:VARYG | season Season 2 s2"},
	{"Jujutsu Kaisen Season 2 [BD 1080p]", "Jujutsu Kaisen", 0, "1080p BluRay unknown | season Season 2 s2"},
	{"Law.and.Order.SVU.S01.1080p.AMZN.WEB-DL.DDP2.0.H.264-NTb", "Law and Order SVU", 0, "1080p WEB-DL x264 audio:DD+ ch:2.0 g:NTb | season Season 1 s1"},
	{"Seven.Worlds.One.Planet.2019.COMPLETE.UHD.BLURAY-GRP", "Seven Worlds One Planet", 2019, "2160p BluRay unknown g:GRP | complete Complete"},
	{"Special.S01.1080p.NF.WEB-DL.DDP5.1.x264-NTG", "Special", 0, "1080p WEB-DL x264 audio:DD+ ch:5.1 g:NTG | season Season 1 s1"},
	{"The Office (US) Season 1", "The Office", 0, "unknown unknown unknown | season Season 1 s1"},
	{"The.Boys.S01.Multi.Quality.WEB", "The Boys", 0, "unknown WEB unknown | season Season 1 s1"},
	{"The.Boys.S04E05.1080p.WEB.H264-SuccessfulCrab", "The Boys", 0, "1080p WEB x264 g:SuccessfulCrab | episode S04E05 s4 e5-5"},
	{"The.Daily.Show.Week.of.2024.03.11.1080p.WEB.h264-EDITH", "The Daily Show Week of", 0, "1080p WEB x264 g:EDITH | episode 2024-03-11 d2024-03-11"},
	{"The.Office.US.S06.1080p.BluRay.x264-GRP", "The Office US", 0, "1080p BluRay x264 g:GRP | season Season 6 s6"},
	{"Trailer.Park.Boys.S01-S12.COMPLETE.1080p.NF.WEB-DL.DDP5.1.x264-GRP", "Trailer Park Boys", 0, "1080p WEB-DL x264 audio:DD+ ch:5.1 g:GRP | multi_season Seasons 1–12 s1,2,3,4,5,6,7,8,9,10,11,12"},
	{"[Anime Time] Spy x Family (Season 01) [1080p][HEVC 10bit x265][AAC][Multi Sub] (Batch)", "Spy x Family", 0, "1080p unknown x265 b10 audio:AAC subs:multi g:Anime Time | season Season 1 s1"},
	{"[Erai-raws] One Piece - 1071 ~ 1075 [1080p][Multiple Subtitle]", "One Piece", 0, "1080p unknown unknown subs:multi g:Erai-raws | multi_episode Episodes 1071–1075 e1071-1075 abs"},
	{"[Erai-raws] Sousou no Frieren - 01 ~ 28 [1080p][Multiple Subtitle]", "Sousou no Frieren", 0, "1080p unknown unknown subs:multi g:Erai-raws | multi_episode Episodes 1–28 e1-28 abs"},
	{"[Erai-raws] Spy x Family Season 2 - 05", "Spy x Family", 0, "unknown unknown unknown g:Erai-raws | episode S02E05 s2 e5-5"},
	{"[Group] Detective Conan 1120-1123 [1080p]", "Detective Conan", 0, "1080p unknown unknown g:Group | multi_episode Episodes 1120–1123 e1120-1123 abs"},
	{"[Group] Jujutsu Kaisen - 29-30 [1080p]", "Jujutsu Kaisen", 0, "1080p unknown unknown g:Group | multi_episode Episodes 29–30 e29-30 abs"},
	{"[Group] Shingeki no Kyojin OAD Collection [BD 1080p]", "Shingeki no Kyojin OAD Collection", 0, "1080p BluRay unknown g:Group"},
	{"[Group] Sousou no Frieren - 14", "Sousou no Frieren", 0, "unknown unknown unknown g:Group | episode Episode 14 e14-14 abs"},
	{"[Group] Spy x Family S3 [1080p]", "Spy x Family", 0, "1080p unknown unknown g:Group | season Season 3 s3"},
	{"[Judas] Sousou no Frieren (Season 1) [1080p][HEVC x265 10bit][Multi-Subs] (Batch)", "Sousou no Frieren", 0, "1080p unknown x265 b10 subs:multi g:Judas | season Season 1 s1"},
	{"[Kaerizaki-Fansub] One Piece (001-061) [DVD 480p]", "One Piece", 0, "480p DVD unknown g:Kaerizaki-Fansub | multi_episode Episodes 1–61 e1-61 abs"},
	{"[SubsPlease] Jujutsu Kaisen (25-28) (1080p) [Batch]", "Jujutsu Kaisen", 0, "1080p unknown unknown g:SubsPlease | multi_episode Episodes 25–28 e25-28 abs"},
	{"[SubsPlease] Sousou no Frieren S2 (01-04) (1080p) [Batch]", "Sousou no Frieren", 0, "1080p unknown unknown g:SubsPlease | multi_episode S02E01–E04 s2 e1-4"},

	// The prototype's fixtures.
	{"[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]", "Sousou no Frieren", 0, "1080p unknown unknown g:SubsPlease | multi_episode Episodes 29–38 e29-38 abs"},
	{"[Erai-raws] Sousou no Frieren - 29 [1080p][Multiple Subtitle][7B1C0F1E].mkv", "Sousou no Frieren", 0, "1080p unknown unknown subs:multi g:Erai-raws | episode Episode 29 e29-29 abs"},
	{"Frieren.Beyond.Journeys.End.S01.COMPLETE.2160p.BluRay.REMUX.HEVC.DTS-HD.MA.5.1-ANiHLS", "Frieren Beyond Journeys End", 0, "2160p REMUX x265 audio:DTS-HD ch:5.1 g:ANiHLS | season Season 1 s1"},
	{"[ASW] Sousou no Frieren - 29 [1080p HEVC x265 10Bit][AAC]", "Sousou no Frieren", 0, "1080p unknown x265 b10 audio:AAC g:ASW | episode Episode 29 e29-29 abs"},
	{"Frieren - 29 [720p] [Judas]", "Frieren", 0, "720p unknown unknown g:Judas | episode Episode 29 e29-29 abs"},
	{"Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", "Severance", 0, "2160p WEB-DL x265 hdr:DV,HDR audio:Atmos,DD+ ch:5.1 g:FLUX | season Season 2 s2"},
	{"Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", "Severance", 0, "2160p WEB-DL x265 hdr:DV,HDR audio:Atmos,DD+ ch:5.1 g:FLUX | episode S02E05 s2 e5-5"},
	{"The.Good.Place.S01.1080p.NF.WEB-DL.DDP5.1.x264-NTb", "The Good Place", 0, "1080p WEB-DL x264 audio:DD+ ch:5.1 g:NTb | season Season 1 s1"},
	{"The.Good.Place.S01-S04.COMPLETE.1080p.BluRay.x265.10bit.AAC5.1-YAWNiX", "The Good Place", 0, "1080p BluRay x265 b10 audio:AAC ch:5.1 g:YAWNiX | multi_season Seasons 1–4 s1,2,3,4"},
	{"The Good Place Season 1 Complete 720p WEB x264 [i_c]", "The Good Place", 0, "720p WEB x264 g:i_c | season Season 1 s1"},
	{"The.Good.Place.S01.2160p.HDR.WEB.H265-CAKES", "The Good Place", 0, "2160p WEB x265 hdr:HDR g:CAKES | season Season 1 s1"},
	{"[EMBER] Frieren (2023) (Season 1) [1080p] [HEVC WEBRip DDP] (Sousou no Frieren)", "Frieren", 2023, "1080p WEBRip x265 audio:DD+ g:EMBER | season Season 1 s1"},
	{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", "Dune Part Two", 2024, "2160p WEB-DL x265 hdr:DV,HDR audio:Atmos,DD+ ch:5.1 g:FLUX"},
	{"Shogun.2024.S01.2160p.DSNP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", "Shogun", 2024, "2160p WEB-DL x265 hdr:DV,HDR audio:Atmos,DD+ ch:5.1 g:FLUX | season Season 1 s1"},
	{"Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT", "Interstellar", 2014, "2160p REMUX x265 hdr:HDR audio:DTS-HD ch:5.1 g:FGT"},
	{"The.Bear.S04.1080p.HULU.WEB-DL.DDP5.1.H.264-NTb", "The Bear", 0, "1080p WEB-DL x264 audio:DD+ ch:5.1 g:NTb | season Season 4 s4"},

	// Movies, and titles that look like tags or numbers.
	{"Interstellar (2014) [1080p] [BluRay] [5.1] [YTS.MX]", "Interstellar", 2014, "1080p BluRay unknown ch:5.1 g:YTS"},
	{"Interstellar.2014.1080p.BluRay.x264.YIFY", "Interstellar", 2014, "1080p BluRay x264 g:YTS"},
	{"Interstellar.2014.1080p.BluRay.x264-[YTS.AM]", "Interstellar", 2014, "1080p BluRay x264 g:YTS"},
	{"Blade.Runner.2049.2017.1080p.BluRay.x264-SPARKS", "Blade Runner 2049", 2017, "1080p BluRay x264 g:SPARKS"},
	{"Blade Runner 2049 (2017) [2160p] [4K] [BluRay] [7.1] [YTS.MX]", "Blade Runner 2049", 2017, "2160p BluRay unknown ch:7.1 g:YTS"},
	{"2001.A.Space.Odyssey.1968.1080p.BluRay.x264-AMIABLE", "2001 A Space Odyssey", 1968, "1080p BluRay x264 g:AMIABLE"},
	{"Charlotte's.Web.2006.1080p.BluRay.x264-GRP", "Charlotte's Web", 2006, "1080p BluRay x264 g:GRP"},
	{"Uncut.Gems.2019.1080p.WEBRip.x264-RARBG", "Uncut Gems", 2019, "1080p WEBRip x264 g:RARBG"},
	{"Cam.2018.720p.NF.WEB-DL.DDP5.1.x264-NTG", "Cam", 2018, "720p WEB-DL x264 audio:DD+ ch:5.1 g:NTG"},
	{"Apollo.13.1995.1080p.BluRay.x264-AMIABLE", "Apollo 13", 1995, "1080p BluRay x264 g:AMIABLE"},
	{"Kill.Bill.Vol.1-2.2003.1080p.BluRay.x264", "Kill Bill Vol 1-2", 2003, "1080p BluRay x264"},
	{"Spider-Man.No.Way.Home.2021.2160p.WEB-DL.DDP5.1.Atmos.HDR10.HEVC-GRP", "Spider-Man No Way Home", 2021, "2160p WEB-DL x265 hdr:HDR10 audio:Atmos,DD+ ch:5.1 g:GRP"},
	{"Movie.2019.1080p.BluRay.x264-SPARKS.mkv", "Movie", 2019, "1080p BluRay x264 g:SPARKS"},

	// Filmed in a cinema, or a screener.
	{"Oppenheimer.2023.HDCAM.x264-GRP", "Oppenheimer", 2023, "unknown CAM x264 g:GRP"},
	{"Oppenheimer.2023.HDCAM-GRP", "Oppenheimer", 2023, "unknown CAM unknown g:GRP"},
	{"Deadpool.and.Wolverine.2024.1080p.HDTS.x264-AOC", "Deadpool and Wolverine", 2024, "1080p CAM x264 g:AOC"},
	{"Joker Folie a Deux 2024 TELESYNC 720p x264 AAC", "Joker Folie a Deux", 2024, "720p CAM x264 audio:AAC"},
	{"Gladiator.II.2024.CAM.x264-NOGRP", "Gladiator II", 2024, "unknown CAM x264 g:NOGRP"},
	{"Dune Part Two 2024 1080p TS x264", "Dune Part Two", 2024, "1080p CAM x264"},
	{"Movie.2024.DVDSCR.XviD-GRP", "Movie", 2024, "unknown CAM other g:GRP"},
	{"Movie.2024.HDTC.720p.x264", "Movie", 2024, "720p CAM x264"},
	{"Movie.2023.SCR.x264-GRP", "Movie", 2023, "unknown CAM x264 g:GRP"},
	{"Movie.2024.TELECINE.x264-GRP", "Movie", 2024, "unknown CAM x264 g:GRP"},

	// Cuts and repacks.
	{"Blade.Runner.1982.Directors.Cut.REPACK.1080p.BluRay.x264-AMIABLE", "Blade Runner", 1982, "1080p BluRay x264 g:AMIABLE ed:Director's Cut repack"},
	{"1917.2019.Extended.2160p.UHD.BluRay.x265-TERMiNAL", "1917", 2019, "2160p BluRay x265 g:TERMiNAL ed:Extended"},
	{"Aliens.1986.Extended.720p.BRRip.x264-LOL", "Aliens", 1986, "720p BluRay x264 g:LOL ed:Extended"},
	{"The.Lord.of.the.Rings.The.Fellowship.of.the.Ring.2001.EXTENDED.2160p.UHD.BluRay.x265.10bit.HDR.TrueHD.7.1.Atmos-DON",
		"The Lord of the Rings The Fellowship of the Ring", 2001, "2160p BluRay x265 b10 hdr:HDR audio:Atmos,TrueHD ch:7.1 g:DON ed:Extended"},
	{"Apocalypse.Now.1979.Final.Cut.1080p.BluRay.x264-GRP", "Apocalypse Now", 1979, "1080p BluRay x264 g:GRP ed:Final Cut"},
	{"Brazil.1985.Criterion.1080p.BluRay.x264-GRP", "Brazil", 1985, "1080p BluRay x264 g:GRP ed:Criterion"},
	{"Terminator.2.1991.Remastered.1080p.BluRay.x264-GRP", "Terminator 2", 1991, "1080p BluRay x264 g:GRP ed:Remastered"},
	{"Interstellar.2014.IMAX.1080p.WEBRip.x264", "Interstellar", 2014, "1080p WEBRip x264 ed:IMAX"},
	{"American.Pie.1999.UNRATED.720p.BluRay.x264-GRP", "American Pie", 1999, "720p BluRay x264 g:GRP ed:Unrated"},
	{"Aliens.1986.Theatrical.1080p.BluRay.x264-GRP", "Aliens", 1986, "1080p BluRay x264 g:GRP ed:Theatrical"},
	{"Movie.2019.Special.Edition.1080p.BluRay.x264-GRP", "Movie", 2019, "1080p BluRay x264 g:GRP ed:Special Edition"},
	{"Movie.2019.Uncut.1080p.BluRay.x264-GRP", "Movie", 2019, "1080p BluRay x264 g:GRP ed:Uncut"},
	{"Show.S01E05.PROPER.720p.HDTV.x264-KILLERS", "Show", 0, "720p HDTV x264 g:KILLERS repack | episode S01E05 s1 e5-5"},
	{"Show.S01E05.REPACK.1080p.WEB.h264-GRP", "Show", 0, "1080p WEB x264 g:GRP repack | episode S01E05 s1 e5-5"},
	// An episode title isn't a cut.
	{"Breaking.Bad.S02E03.The.Final.Cut.720p.BluRay.x264-GRP", "Breaking Bad", 0, "720p BluRay x264 g:GRP | episode S02E03 s2 e3-3"},

	// Series: episodes, multi-episodes, prefixes and dates.
	{"The.Office.US.S07E25-E26.Search.Committee.720p.WEB-DL", "The Office US", 0, "720p WEB-DL unknown | multi_episode S07E25–E26 s7 e25-26"},
	{"The.Office.US.S06E04E05.Niagara.720p", "The Office US", 0, "720p unknown unknown | multi_episode S06E04–E05 s6 e4-5"},
	{"Friends - 2x12 & 2x13 - The One After the Superbowl", "Friends", 0, "unknown unknown unknown | multi_episode S02E12–E13 s2 e12-13"},
	{"Show.S01.E01-E10.1080p.WEB", "Show", 0, "1080p WEB unknown | multi_episode S01E01–E10 s1 e1-10"},
	{"S.W.A.T.2017.S07E01.1080p.WEB.h264-ETHEL", "S.W.A.T", 2017, "1080p WEB x264 g:ETHEL | episode S07E01 s7 e1-1"},
	{"www.UIndex.org    -    Show.S01E05.1080p.WEB.h264-GRP", "Show", 0, "1080p WEB x264 g:GRP | episode S01E05 s1 e5-5"},
	{"[ www.Torrenting.com ] - Show.S01E05.720p.HDTV.x264-GRP", "Show", 0, "720p HDTV x264 g:GRP | episode S01E05 s1 e5-5"},
	{"Show.S01E01.720p.HDTV.x264-KILLERS[eztv]", "Show", 0, "720p HDTV x264 g:KILLERS | episode S01E01 s1 e1-1"},
	{"The.Daily.Show.2024.03.15.1080p.WEB.h264-EDITH", "The Daily Show", 0, "1080p WEB x264 g:EDITH | episode 2024-03-15 d2024-03-15"},
	{"Show.S00E01.Pilot.720p.WEB", "Show", 0, "720p WEB unknown | episode S00E01 s0 e1-1"},
	{"Show.S01E05-S02E03.720p", "Show", 0, "720p unknown unknown | unknown S01E05–S02E03 s1,2"},
	{"Show_Name_S01E01_1080p_WEB", "Show Name", 0, "1080p WEB unknown | episode S01E01 s1 e1-1"},
	{"The Last of Us S01E01 1080p WEB H264-GRP", "The Last of Us", 0, "1080p WEB x264 g:GRP | episode S01E01 s1 e1-1"},
	{"Show.2019.S01E01.1080p.WEB.x264-GRP", "Show", 2019, "1080p WEB x264 g:GRP | episode S01E01 s1 e1-1"},

	// Anime numbering.
	{"One.Piece.E1088.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG", "One Piece", 0, "1080p WEB-DL x264 audio:AAC ch:2.0 g:VARYG | episode Episode 1088 e1088-1088 abs"},
	{"Jujutsu Kaisen Episode 30 [1080p]", "Jujutsu Kaisen", 0, "1080p unknown unknown | episode Episode 30 e30-30 abs"},
	{"[SubsPlease] One Piece - 1071 (1080p) [ABCD1234].mkv", "One Piece", 0, "1080p unknown unknown g:SubsPlease | episode Episode 1071 e1071-1071 abs"},
	{"[Group] One Piece 1071 [1080p]", "One Piece", 0, "1080p unknown unknown g:Group | episode Episode 1071 e1071-1071 abs"},
	{"[SubsPlease] Sousou no Frieren - 28.5 (1080p)", "Sousou no Frieren", 0, "1080p unknown unknown g:SubsPlease | unknown Episode 28.5"},
	{"[Judas] Golden Kamuy (Batch) [1080p]", "Golden Kamuy", 0, "1080p unknown unknown g:Judas | complete Complete"},
	{"[Judas] Golden Kamuy S03 [1080p][HEVC x265 10bit][Dual-Audio][Multi-Subs] (Batch)", "Golden Kamuy", 0, "1080p unknown x265 b10 lang:dual subs:multi g:Judas | season Season 3 s3"},
	{"[Group] Show - 01 [ENG SUB] [720p]", "Show", 0, "720p unknown unknown g:Group | episode Episode 1 e1-1 abs"},

	// Packs.
	{"Friends.S01-S10.COMPLETE.1080p.BluRay.x265-GRP", "Friends", 0, "1080p BluRay x265 g:GRP | multi_season Seasons 1–10 s1,2,3,4,5,6,7,8,9,10"},
	{"Friends Seasons 1-10 Complete 1080p BluRay", "Friends", 0, "1080p BluRay unknown | multi_season Seasons 1–10 s1,2,3,4,5,6,7,8,9,10"},
	{"Breaking.Bad.Complete.Series.S01-S05.1080p.BluRay.x264-GRP", "Breaking Bad", 0, "1080p BluRay x264 g:GRP | complete Complete s1,2,3,4,5"},
	{"The.Wire.S01.S02.S03.720p.BluRay", "The Wire", 0, "720p BluRay unknown | multi_season Seasons 1–3 s1,2,3"},
	{"The.Wire.S01.S03.S05.720p.BluRay", "The Wire", 0, "720p BluRay unknown | multi_season Seasons 1, 3, 5 s1,3,5"},
	{"Show.Name.S01.Specials.720p.WEB", "Show Name", 0, "720p WEB unknown | multi_season Season 1 + Specials s0,1"},
	{"Doctor Who (2005-2022) Complete 1080p", "Doctor Who", 2005, "1080p unknown unknown | complete Complete"},

	// Russian trackers: "Сезон" and "Серии ... из N".
	{"Игра престолов / Game of Thrones / Сезон: 1-8 / Серии: 1-73 из 73 [2011-2019, США, фэнтези, драма, BDRip 1080p] MVO (LostFilm) + Original + Sub (Rus, Eng)",
		"Игра престолов / Game of Thrones", 2011, "1080p BluRay unknown subs:multi | multi_season Seasons 1–8 s1,2,3,4,5,6,7,8"},
	{"Шерлок / Sherlock / Сезон: 1 / Серии: 1-3 из 3 [2010, Великобритания, детектив, BDRip 720p] Dub + Original + Sub (Rus, Eng)",
		"Шерлок / Sherlock", 2010, "720p BluRay unknown subs:multi | season Season 1 s1"},
	{"Мандалорец / The Mandalorian / Сезон: 2 / Серии: 1-5 из 8 [2020, США, фантастика, WEB-DL 1080p] MVO (LostFilm) + Original",
		"Мандалорец / The Mandalorian", 2020, "1080p WEB-DL unknown | multi_episode S02E01–E05 s2 e1-5"},
	{"Дюна: Часть вторая / Dune: Part Two (2024) WEB-DL 2160p | HDR | Dolby Vision | Лицензия", "Дюна: Часть вторая / Dune: Part Two", 2024, "2160p WEB-DL unknown hdr:DV,HDR"},
	{"Игра престолов 1 сезон 1080p", "Игра престолов", 0, "1080p unknown unknown | season Season 1 s1"},

	// Languages and subtitles.
	{"Show.S01E01.FRENCH.720p.HDTV.x264-GRP", "Show", 0, "720p HDTV x264 lang:fr g:GRP | episode S01E01 s1 e1-1"},
	{"Show.S01E01.MULTi.1080p.WEB.H264-GRP", "Show", 0, "1080p WEB x264 lang:multi g:GRP | episode S01E01 s1 e1-1"},
	{"Movie.2019.iTA.ENG.1080p.BluRay.x264-GRP", "Movie", 2019, "1080p BluRay x264 lang:it,en g:GRP"},
	{"Movie 2019 1080p BluRay x264 Dual Audio [Hindi + English]", "Movie", 2019, "1080p BluRay x264 lang:dual,hi,en"},
	{"Movie.2019.1080p.BluRay.DUAL.x264-GRP", "Movie", 2019, "1080p BluRay x264 lang:dual g:GRP"},
	{"Movie.2019.GERMAN.DL.1080p.BluRay.x264-GRP", "Movie", 2019, "1080p BluRay x264 lang:de g:GRP"},
	{"Movie.2019.1080p.WEB-DL.RUS.ENG.x264", "Movie", 2019, "1080p WEB-DL x264 lang:ru,en"},
	{"Movie.2019.1080p.BluRay.x264.VOSTFR-GRP", "Movie", 2019, "1080p BluRay x264 g:GRP"},
	{"Movie (2019) 1080p WEB-DL [Multi-Sub]", "Movie", 2019, "1080p WEB-DL unknown subs:multi"},
	{"Movie.2019.HC.HDRip.x264-GRP", "Movie", 2019, "unknown unknown x264 subs:hardcoded g:GRP"},
	{"Movie.2019.1080p.KORSUB.HDRip", "Movie", 2019, "1080p unknown unknown subs:hardcoded"},

	// Video and audio tags.
	{"Movie.2019.2160p.WEB-DL.DV.HDR10+.DDP5.1.Atmos.H.265-GRP", "Movie", 2019, "2160p WEB-DL x265 hdr:DV,HDR10+ audio:Atmos,DD+ ch:5.1 g:GRP"},
	{"Movie.2019.2160p.BluRay.REMUX.HDR10.HEVC.TrueHD.7.1.Atmos-FGT", "Movie", 2019, "2160p REMUX x265 hdr:HDR10 audio:Atmos,TrueHD ch:7.1 g:FGT"},
	{"Movie.2019.2160p.HEVC.12bit.DoVi.WEB-DL", "Movie", 2019, "2160p WEB-DL x265 b12 hdr:DV"},
	{"Movie.2019.1080p.Dolby.Vision.WEB-DL", "Movie", 2019, "1080p WEB-DL unknown hdr:DV"},
	{"Movie.2019.4K.HDR.x265", "Movie", 2019, "2160p unknown x265 hdr:HDR"},
	{"Movie.2019.1080p.BluRay.DTS.x264-GRP", "Movie", 2019, "1080p BluRay x264 audio:DTS g:GRP"},
	{"Movie.2019.1080p.BluRay.x264.DTS-X.7.1", "Movie", 2019, "1080p BluRay x264 audio:DTS-HD ch:7.1"},
	{"Movie.2019.1080p.BluRay.FLAC.2.0.x264-GRP", "Movie", 2019, "1080p BluRay x264 audio:FLAC ch:2.0 g:GRP"},
	{"Movie.2019.1080p.WEB-DL.Opus.5.1.AV1-GRP", "Movie", 2019, "1080p WEB-DL AV1 audio:Opus ch:5.1 g:GRP"},
	{"Movie.2019.1080p.BluRay.AC3.x264-GRP", "Movie", 2019, "1080p BluRay x264 audio:DD g:GRP"},
	{"Movie.2019.1080p.WEB.DL.DD5.1.H264-GRP", "Movie", 2019, "1080p WEB-DL x264 audio:DD ch:5.1 g:GRP"},
	{"Movie.2019.1080p.WEB-DL.DD+5.1.x265-GRP", "Movie", 2019, "1080p WEB-DL x265 audio:DD+ ch:5.1 g:GRP"},
	{"Movie.2019.720p.HDTV.EAC3.x264", "Movie", 2019, "720p HDTV x264 audio:DD+"},
	{"Movie.2019.1080p.Hi10P.BluRay.FLAC", "Movie", 2019, "1080p BluRay x264 b10 audio:FLAC"},
	{"Movie (2019) 1080p BluRay x264 AAC 6ch", "Movie", 2019, "1080p BluRay x264 audio:AAC ch:5.1"},
	{"Movie.2019.WEB-DLRip.AVC.Rus", "Movie", 2019, "unknown WEBRip x264 lang:ru"},
	{"Movie.2019.1080p.AMZN.WEBRip.DDP5.1.x264-NTG", "Movie", 2019, "1080p WEBRip x264 audio:DD+ ch:5.1 g:NTG"},
	{"Show.S01E01.1080p.AMZN.x264-GRP", "Show", 0, "1080p WEB x264 g:GRP | episode S01E01 s1 e1-1"},
	{"Movie.2019.576p.DVD.x264", "Movie", 2019, "576p DVD x264"},
	{"Movie.2019.480p.DVDRip.XviD", "Movie", 2019, "480p DVD other"},

	// Resolutions and years are never episodes.
	{"Movie.1920x1080.2019.BluRay.x264", "Movie", 2019, "1080p BluRay x264"},
	{"Movie.2019.1920x800.BluRay.x264", "Movie", 2019, "1080p BluRay x264"},
	{"Movie 2019 720x576 DVDRip", "Movie", 2019, "576p DVD unknown"},
	{"Show.S01E01.1280x720.WEB-DL", "Show", 0, "720p WEB-DL unknown | episode S01E01 s1 e1-1"},
	{"", "", 0, "unknown unknown unknown"},
}

// summary renders what Parse read: resolution, source and codec, then the
// attributes that are set, then the pack.
func summary(r Release) string {
	parts := []string{r.Resolution, r.Source, r.Codec}
	add := func(label, v string) {
		if v != "" {
			parts = append(parts, label+v)
		}
	}
	if r.BitDepth != 0 {
		parts = append(parts, "b"+strconv.Itoa(r.BitDepth))
	}
	add("hdr:", strings.Join(r.HDR, ","))
	add("audio:", strings.Join(r.Audio, ","))
	add("ch:", r.Channels)
	add("lang:", strings.Join(r.Languages, ","))
	add("subs:", r.Subs)
	add("g:", r.Group)
	add("ed:", r.Edition)
	if r.Repack {
		parts = append(parts, "repack")
	}
	s := strings.Join(parts, " ")
	if p := r.Pack; p != nil {
		s += " | " + p.Type + " " + p.Label
		if len(p.Seasons) > 0 {
			seasons := make([]string, len(p.Seasons))
			for i, n := range p.Seasons {
				seasons[i] = strconv.Itoa(n)
			}
			s += " s" + strings.Join(seasons, ",")
		}
		if p.Episodes != nil {
			s += fmt.Sprintf(" e%d-%d", p.Episodes.From, p.Episodes.To)
		}
		if p.Absolute {
			s += " abs"
		}
		if p.Date != "" {
			s += " d" + p.Date
		}
	}
	return s
}

func TestParse(t *testing.T) {
	for _, tt := range parseCases {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.name)
			if got := summary(r); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
			if r.Title != tt.title {
				t.Errorf("Title = %q, want %q", r.Title, tt.title)
			}
			if r.Year != tt.year {
				t.Errorf("Year = %d, want %d", r.Year, tt.year)
			}
		})
	}
}

// TestParse_AllowedValues checks every case against PLAN §6's allowed
// values.
func TestParse_AllowedValues(t *testing.T) {
	allowed := map[string][]string{
		"resolution": {"2160p", "1080p", "720p", "576p", "480p", "unknown"},
		"source":     {"REMUX", "BluRay", "WEB-DL", "WEBRip", "WEB", "HDTV", "DVD", "CAM", "unknown"},
		"codec":      {"x265", "x264", "AV1", "other", "unknown"},
		"hdr":        hdrOrder,
		"audio":      audioOrder,
		"channels":   {"7.1", "5.1", "2.0", ""},
		"subs":       {"multi", "hardcoded", ""},
		"pack":       {PackEpisode, PackMultiEpisode, PackSeason, PackMultiSeason, PackComplete, PackUnknown},
	}
	check := func(name, field string, values ...string) {
		for _, v := range values {
			if !slices.Contains(allowed[field], v) {
				t.Errorf("%s: %s %q isn't allowed", name, field, v)
			}
		}
	}
	for _, tt := range parseCases {
		r := Parse(tt.name)
		check(tt.name, "resolution", r.Resolution)
		check(tt.name, "source", r.Source)
		check(tt.name, "codec", r.Codec)
		check(tt.name, "hdr", r.HDR...)
		check(tt.name, "audio", r.Audio...)
		check(tt.name, "channels", r.Channels)
		check(tt.name, "subs", r.Subs)
		if r.BitDepth != 0 && r.BitDepth != 10 && r.BitDepth != 12 {
			t.Errorf("%s: bit depth %d isn't allowed", tt.name, r.BitDepth)
		}
		for _, l := range r.Languages {
			if l != "multi" && l != "dual" && len(l) != 2 {
				t.Errorf("%s: language %q isn't multi, dual or ISO 639-1", tt.name, l)
			}
		}
		if r.Pack != nil {
			check(tt.name, "pack", r.Pack.Type)
		}
	}
}

func TestReleaseJSON(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Severance.S02E05.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX",
			`{"resolution":"2160p","source":"WEB-DL","codec":"x265","bit_depth":null,"hdr":["DV","HDR"],"audio":["Atmos","DD+"],"channels":"5.1","languages":[],"subs":null,"group":"FLUX","edition":null,"repack":false,"pack":{"type":"episode","label":"S02E05","seasons":[2],"episodes":{"from":5,"to":5}}}`},
		{"Interstellar.2014.1080p.BluRay.x264",
			`{"resolution":"1080p","source":"BluRay","codec":"x264","bit_depth":null,"hdr":[],"audio":[],"channels":null,"languages":[],"subs":null,"group":null,"edition":null,"repack":false,"pack":null}`},
		{"[Judas] Golden Kamuy S03 [1080p][HEVC x265 10bit][Dual-Audio][Multi-Subs] (Batch)",
			`{"resolution":"1080p","source":"unknown","codec":"x265","bit_depth":10,"hdr":[],"audio":[],"channels":null,"languages":["dual"],"subs":"multi","group":"Judas","edition":null,"repack":false,"pack":{"type":"season","label":"Season 3","seasons":[3],"episodes":null}}`},
		// Absolute isn't part of the contract.
		{"[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]",
			`{"resolution":"1080p","source":"unknown","codec":"unknown","bit_depth":null,"hdr":[],"audio":[],"channels":null,"languages":[],"subs":null,"group":"SubsPlease","edition":null,"repack":false,"pack":{"type":"multi_episode","label":"Episodes 29–38","seasons":[],"episodes":{"from":29,"to":38}}}`},
		{"Blade.Runner.1982.Directors.Cut.REPACK.1080p.BluRay.x264-AMIABLE",
			`{"resolution":"1080p","source":"BluRay","codec":"x264","bit_depth":null,"hdr":[],"audio":[],"channels":null,"languages":[],"subs":null,"group":"AMIABLE","edition":"Director's Cut","repack":true,"pack":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(Parse(tt.name))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestDisplayTitle(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", "Dune Part Two 2024 2160p WEB-DL DDP5.1 Atmos DV HDR H.265-FLUX"},
		{"Movie.2019.1080p.WEB-DL.DD+5.1.x265-GRP", "Movie 2019 1080p WEB-DL DD+5.1 x265-GRP"},
		{"Movie.2019.1080p.BluRay.TrueHD.7.1.Atmos.AAC2.0.x264", "Movie 2019 1080p BluRay TrueHD 7.1 Atmos AAC2.0 x264"},
		{"Interstellar.2014.1080p.BluRay.x264-[YTS.AM]", "Interstellar 2014 1080p BluRay x264-[YTS.AM]"},
		{"S.W.A.T.2017.S07E01.1080p.WEB.h264-ETHEL", "S.W.A.T 2017 S07E01 1080p WEB h264-ETHEL"},
		{"The.Daily.Show.2024.03.15.1080p.WEB.h264-EDITH", "The Daily Show 2024.03.15 1080p WEB h264-EDITH"},
		{"Show_Name_S01E01_1080p_WEB", "Show Name S01E01 1080p WEB"},
		{"Movie.2019.1080p.BluRay.x264-SPARKS.mkv", "Movie 2019 1080p BluRay x264-SPARKS"},
		{"Movie.Name.2019.1080p.WEB-DL.x264 [Group]", "Movie Name 2019 1080p WEB-DL x264 [Group]"},
		// Names written with spaces are already readable.
		{"Interstellar (2014) [1080p] [BluRay] [5.1] [YTS.MX]", "Interstellar (2014) [1080p] [BluRay] [5.1] [YTS.MX]"},
		{"The Good Place Season 1 Complete 720p WEB x264 [i_c]", "The Good Place Season 1 Complete 720p WEB x264 [i_c]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.name).DisplayTitle; got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
