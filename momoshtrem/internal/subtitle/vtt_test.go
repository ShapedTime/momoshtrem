package subtitle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// bom is the UTF-8 byte order mark HDRezka's English files start with.
const bom = "\xef\xbb\xbf"

// crlf writes lines with Windows line ends.
func crlf(lines ...string) string { return strings.Join(lines, "\r\n") }

func TestVTTToSRT(t *testing.T) {
	tests := []struct {
		name    string
		vtt     string
		want    string
		wantErr error
	}{
		{
			// The shape of HDRezka's English tracks: a BOM, CRLF and cues
			// without identifiers.
			name: "HDRezka: BOM, CRLF, no identifiers",
			vtt: bom + crlf("WEBVTT", "",
				"00:01:14.908 --> 00:01:16.988", "My father grew corn.", "",
				"00:01:33.302 --> 00:01:35.852", "- Pull up!", "- I've got it.", "",
				"01:48:02.050 --> 01:48:04.120", "Wake up.", "", ""),
			want: "1\n00:01:14,908 --> 00:01:16,988\nMy father grew corn.\n\n" +
				"2\n00:01:33,302 --> 00:01:35,852\n- Pull up!\n- I've got it.\n\n" +
				"3\n01:48:02,050 --> 01:48:04,120\nWake up.\n\n",
		},
		{
			// The shape of its Russian and Ukrainian tracks: LF, numbered
			// cues, italics and bold.
			name: "HDRezka: numbered cues, <i> and <b>",
			vtt: "WEBVTT\n\n" +
				"1\n00:01:06.000 --> 00:01:13.400\n<b>ИНТЕРСТЕЛЛАР</b>\n\n" +
				"2\n00:01:15.122 --> 00:01:17.322\n<i>Мой отец был фермером.</i>\n\n" +
				"3\n00:01:18.298 --> 00:01:20.755\nКак и все тогда.\n",
			want: "1\n00:01:06,000 --> 00:01:13,400\n<b>ИНТЕРСТЕЛЛАР</b>\n\n" +
				"2\n00:01:15,122 --> 00:01:17,322\n<i>Мой отец был фермером.</i>\n\n" +
				"3\n00:01:18,298 --> 00:01:20,755\nКак и все тогда.\n\n",
		},
		{
			name: "header text and metadata",
			vtt:  "WEBVTT - Pilot\nKind: captions\nLanguage: en\n\n00:00:01.000 --> 00:00:02.000\nHello.\n",
			want: "1\n00:00:01,000 --> 00:00:02,000\nHello.\n\n",
		},
		{
			name: "a cue right under the header",
			vtt:  "WEBVTT\n00:00:01.000 --> 00:00:02.000\nHello.\n",
			want: "1\n00:00:01,000 --> 00:00:02,000\nHello.\n\n",
		},
		{
			name: "NOTE, STYLE and REGION blocks",
			vtt: "WEBVTT\n\n" +
				"NOTE This file was made by hand\nand the note goes on.\n\n" +
				"STYLE\n::cue {\n  color: yellow;\n}\n\n" +
				"REGION\nid:bill\nwidth:40%\n\n" +
				"NOTE\n\n" +
				"00:00:01.000 --> 00:00:02.000\nKept.\n\n" +
				"NOTES are not a block keyword\n00:00:03.000 --> 00:00:04.000\nAn identifier, then a cue.\n",
			want: "1\n00:00:01,000 --> 00:00:02,000\nKept.\n\n" +
				"2\n00:00:03,000 --> 00:00:04,000\nAn identifier, then a cue.\n\n",
		},
		{
			name: "identifiers of every kind",
			vtt:  "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:02.000\nOne.\n\n42\n00:00:03.000 --> 00:00:04.000\nTwo.\n",
			want: "1\n00:00:01,000 --> 00:00:02,000\nOne.\n\n2\n00:00:03,000 --> 00:00:04,000\nTwo.\n\n",
		},
		{
			name: "timestamps without hours, and long ones",
			vtt:  "WEBVTT\n\n01:02.500 --> 01:04.000\nShort.\n\n123:00:00.007 --> 123:00:01.010\nLong.\n",
			want: "1\n00:01:02,500 --> 00:01:04,000\nShort.\n\n2\n123:00:00,007 --> 123:00:01,010\nLong.\n\n",
		},
		{
			name: "cue settings after the timing",
			vtt:  "WEBVTT\n\n00:00:01.000 --> 00:00:02.000 line:0 position:20% size:60% align:start\nTop left.\n",
			want: "1\n00:00:01,000 --> 00:00:02,000\nTop left.\n\n",
		},
		{
			name: "classes, voices and karaoke timestamps go; <i>, <b> and <u> stay",
			vtt: "WEBVTT\n\n00:00:01.000 --> 00:00:05.000\n" +
				"<v Walter>Say my <c.yellow>name</c>.</v>\n" +
				"<v.loud Jesse>Yo<00:00:03.000> <i.whisper>Mr.</i> <u>White</u>!</v>\n" +
				"<lang en><ruby>漢<rt>kan</rt></ruby></lang> <B>not a tag</B>\n",
			want: "1\n00:00:01,000 --> 00:00:05,000\nSay my name.\nYo <i>Mr.</i> <u>White</u>!\n漢kan not a tag\n\n",
		},
		{
			name: "entities",
			vtt:  "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nTom &amp; Jerry &lt;3 &gt;_&lt;\nnon&nbsp;breaking &#39;quoted&#39;\n",
			want: "1\n00:00:01,000 --> 00:00:02,000\nTom & Jerry <3 >_<\nnon breaking 'quoted'\n\n",
		},
		{
			name: "empty cues are dropped and the rest renumbered",
			vtt: "WEBVTT\n\n" +
				"00:00:01.000 --> 00:00:02.000\n\n" +
				"00:00:03.000 --> 00:00:04.000\n<i></i>\n\n" +
				"00:00:05.000 --> 00:00:06.000\n&nbsp;\n<v Narrator></v>\nOnly this line.\n\n" +
				"00:00:07.000 --> 00:00:08.000\n  Trimmed.  \n",
			want: "1\n00:00:05,000 --> 00:00:06,000\nOnly this line.\n\n2\n00:00:07,000 --> 00:00:08,000\nTrimmed.\n\n",
		},
		{
			name: "malformed timings are skipped",
			vtt: "WEBVTT\n\n" +
				"00:00:01 --> 00:00:02.000\nNo milliseconds.\n\n" +
				"00:00:03.000 -->\nNo end.\n\n" +
				"an identifier\nwithout a timing\n\n" +
				"00:00:05,500 --> 00:00:06.5\nA comma and one digit pass.\n",
			want: "1\n00:00:05,500 --> 00:00:06,500\nA comma and one digit pass.\n\n",
		},
		{
			name: "old Mac line ends",
			vtt:  "WEBVTT\r\r00:00:01.000 --> 00:00:02.000\rOne.\rTwo.\r",
			want: "1\n00:00:01,000 --> 00:00:02,000\nOne.\nTwo.\n\n",
		},
		{name: "an HTML page", vtt: "<!DOCTYPE html><title>Just a moment...</title>", wantErr: subtitle.ErrNotVTT},
		{name: "a header that only starts like one", vtt: "WEBVTTX\n\n00:00:01.000 --> 00:00:02.000\nHi.\n", wantErr: subtitle.ErrNotVTT},
		{name: "an SRT file", vtt: "1\n00:00:01,000 --> 00:00:02,000\nHi.\n", wantErr: subtitle.ErrNotVTT},
		{name: "empty", vtt: "", wantErr: subtitle.ErrNotVTT},
		{name: "no cues", vtt: bom + "WEBVTT\n\nNOTE nothing here\n", wantErr: subtitle.ErrNoCues},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := subtitle.VTTToSRT([]byte(tt.vtt))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("SRT =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
