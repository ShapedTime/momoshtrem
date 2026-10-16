package token

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// describe renders tokens compactly: "w:the w:boys se:1/5 res:1080 g:NTb".
func describe(toks []Token) string {
	parts := make([]string, 0, len(toks))
	for _, t := range toks {
		var s string
		switch t.Kind {
		case Word:
			s = "w:" + t.Norm
		case Num:
			s = "n:" + strconv.Itoa(t.N)
			if t.Half {
				s += ".5"
			}
			if t.Ver > 0 {
				s += "v" + strconv.Itoa(t.Ver)
			}
			if t.Bracket {
				s = "[" + s + "]"
			}
		case SxxEyy, NxNN:
			prefix := "se:"
			if t.Kind == NxNN {
				prefix = "nx:"
			}
			s = prefix + strconv.Itoa(t.Season) + "/" + joinInts(t.Eps)
			if t.Ver > 0 {
				s += "v" + strconv.Itoa(t.Ver)
			}
		case Season:
			s = "s:" + strconv.Itoa(t.Season)
			if t.LastSeason != t.Season {
				s += "-" + strconv.Itoa(t.LastSeason)
			}
		case Episode:
			s = "e:" + joinInts(t.Eps)
		case Date:
			s = "d:" + strings.Join(t.Dates, "|")
			if t.Date == "" {
				s = "d:?"
			}
		case Res:
			s = "res:" + strconv.Itoa(t.N)
		case Codec:
			s = "codec:" + t.Norm
		case Source:
			s = "src:" + t.Norm
		case CRC:
			s = "crc"
		case Year:
			s = "y:" + strconv.Itoa(t.N)
		case Group:
			s = "g:" + t.Text
		case Version:
			s = "v:" + strconv.Itoa(t.N)
		case Range:
			s = fmt.Sprintf("r:%d-%d", t.Lo, t.Hi)
		case Dash:
			s = "-"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

func joinInts(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ",")
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"scene episode", "The.Boys.S04E01.Department.of.Dirty.Tricks.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb",
			"w:the w:boys se:4/1 w:department w:of w:dirty w:tricks res:1080 src:amzn src:web-dl codec:ddp5.1 codec:h.264 g:NTb"},
		{"multi-episode list", "The.Office.US.S06E04E05.Niagara", "w:the w:office w:us se:6/4,5 w:niagara"},
		{"multi-episode dash", "The.Office.US.S07E25-E26.Search.Committee", "w:the w:office w:us se:7/25,26 w:search w:committee"},
		{"multi-episode short range", "Friends.S04E23-24.The.One", "w:friends se:4/23,24 w:the w:one"},
		{"episode range of three", "Show.S01E01-03.720p", "w:show se:1/1,2,3 res:720"},
		{"episode then dash resolution", "Show.S01E01-1080p", "w:show se:1/1 res:1080"},
		{"season and episode apart", "Show S01 E05", "w:show se:1/5"},
		{"season and episode words", "Show Season 1 Episode 5", "w:show se:1/5"},
		{"episode after episode", "Show.S01E01.E02.720p", "w:show se:1/1,2 res:720"},
		{"nxnn", "Friends - 2x01 - The One", "w:friends - nx:2/1 - w:the w:one"},
		{"nxnn range", "Friends - 2x12-2x13 - The One", "w:friends - nx:2/12,13 - w:the w:one"},
		{"resolution is not nxnn", "Show.S02E05.1920x1080.WEB.x264-GRP", "w:show se:2/5 res:1080 src:web codec:x264 g:GRP"},
		{"resolution alone", "Frieren.1920x1080", "w:frieren res:1080"},
		{"year", "The.Boys.2019.S01E01", "w:the w:boys y:2019 se:1/1"},
		{"year title", "1923.S01E01.1923.1080p", "y:1923 se:1/1 y:1923 res:1080"},
		{"number title", "24.S01E01.12.00.AM-1.00.AM.720p", "n:24 se:1/1 n:12 n:0 w:am n:1 n:0 w:am res:720"},
		{"dashed number title", "9-1-1.S07E01.Abandon.Ship", "n:9 n:1 n:1 se:7/1 w:abandon w:ship"},
		{"fansub", "[SubsPlease] Sousou no Frieren - 05 (1080p) [8C4F1A2B]",
			"g:SubsPlease w:sousou w:no w:frieren - n:5 res:1080 crc"},
		{"fansub version", "[Erai-raws] Sousou no Frieren - 14v2 [1080p][Multiple Subtitle][BBBB0002]",
			"g:Erai-raws w:sousou w:no w:frieren - n:14v2 res:1080 codec:multiple subtitle crc"},
		{"half episode", "[SubsPlease] Sousou no Frieren - 28.5 (1080p)", "g:SubsPlease w:sousou w:no w:frieren - n:28.5 res:1080"},
		{"season token and number", "[SubsPlease] Spy x Family S2 - 05 (1080p)", "g:SubsPlease w:spy w:x w:family s:2 - n:5 res:1080"},
		{"season word and number", "[SubsPlease] Spy x Family Season 3 - 01", "g:SubsPlease w:spy w:x w:family s:3 - n:1"},
		{"ordinal season", "[Group] Show 2nd Season - 05", "g:Group w:show s:2 - n:5"},
		{"bracketed episode", "[Kamigami] Spy x Family [12][1080p][x264 AAC][CHS]",
			"g:Kamigami w:spy w:x w:family [n:12] res:1080 codec:x264 codec:aac w:chs"},
		{"tilde range", "[Erai-raws] Sousou no Frieren - 01 ~ 28 [1080p]", "g:Erai-raws w:sousou w:no w:frieren - r:1-28 res:1080"},
		{"bracketed range", "[SubsPlease] Jujutsu Kaisen (25-28) (1080p) [Batch]", "g:SubsPlease w:jujutsu w:kaisen r:25-28 res:1080 w:batch"},
		{"dash range", "[Group] Jujutsu Kaisen - 01-02 [BD 1080p]", "g:Group w:jujutsu w:kaisen - r:1-2 src:bd res:1080"},
		{"season range", "Trailer.Park.Boys.S01-S12.COMPLETE.1080p", "w:trailer w:park w:boys s:1-12 w:complete res:1080"},
		{"season word range", "Breaking Bad (2008) Season 1-5 Complete", "w:breaking w:bad y:2008 s:1-5 w:complete"},
		{"series word", "Doctor Who (2005) Series 1 DVDRip", "w:doctor w:who y:2005 s:1 src:dvdrip"},
		{"episode word", "Jujutsu Kaisen Episode 30 [1080p]", "w:jujutsu w:kaisen e:30 res:1080"},
		{"episode keyword is not a marker before a word", "One Piece - Episode of Nami", "w:one w:piece - w:episode w:of w:nami"},
		{"episode token", "One.Piece.E1088.1080p.CR.WEB-DL.AAC2.0.H.264-VARYG",
			"w:one w:piece e:1088 res:1080 src:cr src:web-dl codec:aac2.0 codec:h.264 g:VARYG"},
		{"dotted date", "The.Daily.Show.2024.03.14.Hasan.Minhaj.1080p", "w:the w:daily w:show d:2024-03-14 w:hasan w:minhaj res:1080"},
		{"iso date", "The Daily Show 2024-03-14 Hasan Minhaj 720p", "w:the w:daily w:show d:2024-03-14 w:hasan w:minhaj res:720"},
		{"compact date", "The.Daily.Show.20240312.720p", "w:the w:daily w:show d:2024-03-12 res:720"},
		{"invalid date stays numbers", "Show.2024.13.45", "w:show y:2024 n:13 n:45"},
		{"audio after codec", "[Group] Jujutsu Kaisen - 47 [1080p][AAC 2.0]", "g:Group w:jujutsu w:kaisen - n:47 res:1080 codec:aac codec:2.0"},
		{"dts-hd ma", "Friends.S03E05.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP",
			"w:friends se:3/5 res:1080 src:bluray codec:dts-hd codec:ma codec:5.1 codec:x264 g:GRP"},
		{"ten-bit", "[Group] Sousou no Frieren - 12 (1080p HEVC 10-bit AAC) [ABCD1234]",
			"g:Group w:sousou w:no w:frieren - n:12 res:1080 codec:hevc codec:10-bit codec:aac crc"},
		{"group and sample", "the.boys.s04e01.1080p.amzn.web-dl-ntb-sample", "w:the w:boys se:4/1 res:1080 src:amzn src:web-dl g:ntb w:sample"},
		{"cam source and group", "Movie.2024.HDCAM-GRP", "w:movie y:2024 src:hdcam g:GRP"},
		{"ampersand", "Law & Order - Special Victims Unit - 1x04 - Hysteria",
			"w:law w:and w:order - w:special w:victims w:unit - nx:1/4 - w:hysteria"},
		{"plex style", "The Boys (2019) - S02E03 - Over the Hill [WEBDL-1080p]", "w:the w:boys y:2019 - se:2/3 - w:over w:the w:hill src:webdl res:1080"},
		{"leading number", "01 - Pilot", "n:1 - w:pilot"},
		{"leading number with dots", "03 - ...And the Bag's in the River", "n:3 - w:and w:the w:bag's w:in w:the w:river"},
		{"concatenated number", "the.office.101.dvdrip", "w:the w:office n:101 src:dvdrip"},
		{"filename-like episode title", "Mr.Robot.S01E01.eps1.0_hellofriend.mov.1080p.BluRay.x264-GRP",
			"w:mr w:robot se:1/1 w:eps1 n:0 w:hellofriend w:mov res:1080 src:bluray codec:x264 g:GRP"},
		{"sxxeyy version", "Show.S01E05v2.720p", "w:show se:1/5v2 res:720"},
		{"nxnn joined by &", "Friends - 2x12 & 2x13 - The One", "w:friends - nx:2/12,13 - w:the w:one"},
		{"sxxeyy joined by &", "Friends - S02E12 & S02E13 - The One", "w:friends - se:2/12,13 - w:the w:one"},
		{"sxxeyy side by side", "Friends.S02E12.S02E13.The.One", "w:friends se:2/12,13 w:the w:one"},
		{"nxnn joined by +", "The Office - 6x04 + 6x05 - Niagara", "w:the w:office - nx:6/4,5 - w:niagara"},
		{"nxnn joined by a comma", "Friends - 2x12, 2x13 - The One", "w:friends - nx:2/12,13 - w:the w:one"},
		{"nxnn joined by and", "Friends - 2x12 and 2x13", "w:friends - nx:2/12,13"},
		{"plus inside a tag", "Friends S02E12+E13 The One", "w:friends se:2/12,13 w:the w:one"},
		{"plus with the season", "Friends.S02E12+S02E13", "w:friends se:2/12,13"},
		{"different seasons stay apart", "Show.S01E10.S02E01", "w:show se:1/10 se:2/1"},
		{"an earlier episode stays apart", "Show.S01E05.E03", "w:show se:1/5 e:3"},
		{"day first date", "The.Daily.Show.13.03.2024", "w:the w:daily w:show d:2024-03-13"},
		{"either-way date", "The.Daily.Show.03.04.2024", "w:the w:daily w:show d:2024-04-03|2024-03-04"},
		{"dashed day first date", "Show 13-03-2024", "w:show d:2024-03-13"},
		{"two-digit year date", "The.Daily.Show.24.03.13", "w:the w:daily w:show d:?"},
		{"version in brackets", "[Group] Frieren - 14 (v2) [1080p]", "g:Group w:frieren - n:14 v:2 res:1080"},
		{"4k", "Show.S01E01.4K.HDR", "w:show se:1/1 res:2160 codec:hdr"},
		{"ncop", "[Judas] Sousou no Frieren - NCOP01", "g:Judas w:sousou w:no w:frieren - w:ncop01"},
		{"oad number", "[Group] Shingeki no Kyojin - OAD 01 - Ilse's Notebook [BD 1080p]",
			"g:Group w:shingeki w:no w:kyojin - w:oad n:1 - w:ilse's w:notebook src:bd res:1080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describe(Tokenize(tt.in)); got != tt.want {
				t.Errorf("Tokenize(%q)\n got  %s\n want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestTokenizePositions(t *testing.T) {
	in := "[SubsPlease] Frieren - 05 (1080p)"
	for _, tok := range Tokenize(in) {
		if !strings.HasPrefix(in[tok.Pos:], tok.Text) {
			t.Errorf("token %q at %d, input there is %q", tok.Text, tok.Pos, in[tok.Pos:])
		}
	}
}

func TestKindString(t *testing.T) {
	if Res.String() != "res" || Kind(200).String() != "kind(200)" {
		t.Errorf("Kind.String: %q %q", Res.String(), Kind(200).String())
	}
}
