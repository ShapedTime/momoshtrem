package subtitle

import "testing"

func TestParseRelease(t *testing.T) {
	tests := []struct {
		name       string
		wantRes    string
		wantSource string
		wantGroup  string
		wantLabel  string
		wantEd     string
		wantRepack bool
	}{
		{"Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX", "2160p", "web", "flux", "FLUX", "", false},
		{"Severance.S02E05.1080p.ATVP.WEB-DL.DDP5.1.H.264-NTb", "1080p", "web", "ntb", "NTb", "", false},
		{"Interstellar (2014) [1080p] [BluRay] [5.1] [YTS.MX]", "1080p", "bluray", "yts", "YTS", "", false},
		{"Interstellar.2014.1080p.BluRay.x264.YIFY", "1080p", "bluray", "yts", "YTS", "", false},
		{"Interstellar.2014.1080p.BluRay.x264-[YTS.AM].srt", "1080p", "bluray", "yts", "YTS", "", false},
		{"Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT", "2160p", "bluray", "fgt", "FGT", "", false},
		{"[SubsPlease] Sousou no Frieren - 28 (1080p) [A1B2C3D4].mkv", "1080p", "", "subsplease", "SubsPlease", "", false},
		{"Severance.S02E05.720p.HDTV.x264-SYNCOPY", "720p", "hdtv", "syncopy", "SYNCOPY", "", false},
		{"Aliens.1986.Extended.720p.BRRip.x264-LOL", "720p", "bluray", "lol", "LOL", "extended", false},
		{"Blade.Runner.1982.Directors.Cut.REPACK.1080p.BluRay.x264-AMIABLE", "1080p", "bluray", "amiable", "AMIABLE", "directors", true},
		{"Interstellar.2014.IMAX.1080p.WEBRip.x264", "1080p", "web", "", "", "imax", false},
		{"Movie.2014.1080p.WEB-DL.DD5.1.H264", "1080p", "web", "", "", "", false},
		{"Movie.2019.1080p.BluRay.DTS-HD", "1080p", "bluray", "", "", "", false},
		{"Interstellar 2014 HDCAM", "", "cam", "", "", "", false},
		{"", "", "", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ParseRelease(tt.name)
			if r.Resolution != tt.wantRes {
				t.Errorf("Resolution = %q, want %q", r.Resolution, tt.wantRes)
			}
			if r.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", r.Source, tt.wantSource)
			}
			if r.Group != tt.wantGroup {
				t.Errorf("Group = %q, want %q", r.Group, tt.wantGroup)
			}
			if got := r.groupLabel(); got != tt.wantLabel {
				t.Errorf("groupLabel = %q, want %q", got, tt.wantLabel)
			}
			if r.Edition != tt.wantEd {
				t.Errorf("Edition = %q, want %q", r.Edition, tt.wantEd)
			}
			if r.Repack != tt.wantRepack {
				t.Errorf("Repack = %v, want %v", r.Repack, tt.wantRepack)
			}
		})
	}
}

func TestParseRelease_NormalizedDropsExtensionAndPunctuation(t *testing.T) {
	a := ParseRelease("Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv")
	b := ParseRelease("Dune Part Two 2024 2160p WEB-DL DDP5 1 Atmos DV HDR H 265-FLUX")
	if a.Normalized != b.Normalized {
		t.Errorf("Normalized differs:\n%q\n%q", a.Normalized, b.Normalized)
	}
}

func TestAssignedRelease(t *testing.T) {
	const hash = "xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name, magnet, file, want string
	}{
		{"magnet display name", "magnet:?" + hash + "&dn=Interstellar+%282014%29+%5B1080p%5D+%5BYTS.MX%5D&tr=udp%3A%2F%2Ft",
			"Interstellar (2014) [1080p] [YTS.MX]/Interstellar.2014.1080p.BluRay.x264.YIFY.mp4", "Interstellar (2014) [1080p] [YTS.MX]"},
		{"no dn: torrent folder", "magnet:?" + hash,
			"Severance.S02.2160p.ATVP.WEB-DL-FLUX/Severance.S02E05.2160p.mkv", "Severance.S02.2160p.ATVP.WEB-DL-FLUX"},
		{"no dn, single file: file name", "magnet:?" + hash,
			"Dune.Part.Two.2024.2160p.WEB-DL-FLUX.mkv", "Dune.Part.Two.2024.2160p.WEB-DL-FLUX"},
		{"blank dn", "magnet:?" + hash + "&dn=", "Movie.2020.1080p.mkv", "Movie.2020.1080p"},
		{"nothing", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssignedRelease(tt.magnet, tt.file); got != tt.want {
				t.Errorf("AssignedRelease = %q, want %q", got, tt.want)
			}
		})
	}
}
