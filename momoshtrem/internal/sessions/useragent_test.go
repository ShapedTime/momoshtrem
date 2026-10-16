package sessions

import "testing"

func TestParseUserAgent(t *testing.T) {
	tests := []struct {
		name, ua       string
		client, device string // "" for nil
	}{
		{"no User-Agent", "", "", ""},
		{"blank", "   ", "", ""},
		// Infuse names its connection type since 7.7 (Firecore's support
		// article, "Connection Info for Emby, Jellyfin, and Plex").
		{"Infuse direct mode", "Infuse-Direct/7.7", "Infuse", ""},
		{"Infuse library mode", "Infuse-Library/7.7.5", "Infuse", ""},
		{"Infuse download", "Infuse-Download/8.1.7", "Infuse", ""},
		{"Infuse on an Apple TV", "Infuse-Direct/7.7 (AppleTV)", "Infuse", "Apple TV"},
		{"Infuse before 7.7, CFNetwork's default", "Infuse/5205 CFNetwork/1494.0.7 Darwin/23.4.0", "Infuse", ""},
		// The AFNetworking default format, as Channels sends it below.
		{"Infuse on an iPhone", "Infuse/7.6.6 (iPhone; iOS 17.1.2; Scale/3.00)", "Infuse", "iPhone"},
		{"VLC desktop", "VLC/3.0.20 LibVLC/3.0.20", "VLC", ""},
		// From a Universal Media Server log.
		{"VLC on an Apple TV", "VLC-TV/102.1 CFNetwork/758.2.8 Darwin/15.0.0", "VLC", "Apple TV"},
		{"Safari on a Mac",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15",
			"Safari", "Mac"},
		{"Safari on an iPhone",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
			"Safari", "iPhone"},
		{"Safari on an iPad",
			"Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1",
			"Safari", "iPad"},
		{"Chrome says Safari too",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			"Unknown", "Mac"},
		{"Chrome on Android",
			"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
			"Unknown", "Android"},
		{"Kodi on a Shield",
			"Kodi/20.2 (Linux; Android 11.0; SHIELD Android TV Build/RQ1A.210105.003) Android/11.0.0 Sys_CPU/aarch64 App_Bitness/64 Version/20.2-(20.2.0)-Git:20230629-5f418d0b13",
			"Kodi", "Android"},
		{"Kodi on Windows",
			"Kodi/21.0 (Windows NT 10.0.22631; Win64; x64) App_Bitness/64 Version/21.0-(21.0.0)-Git:20240406-5ba8bbe8a4",
			"Kodi", "Windows"},
		{"Kodi on Linux", "Kodi/19.4 (X11; Linux x86_64) Ubuntu/22.04 App_Bitness/64 Version/19.4-(19.4.0)-Git:20220303-c5d37dd6f4",
			"Kodi", "Linux"},
		// Apple TV apps outside the list (community.getchannels.com).
		{"AVPlayer on an Apple TV", "AppleCoreMedia/1.0.0.21L227 (Apple TV; U; CPU OS 17_4 like Mac OS X; en_us)", "Unknown", "Apple TV"},
		{"Channels on an Apple TV", "Channels/7.0.2 (Apple TV; iOS 26.3; Scale/2.00)", "Unknown", "Apple TV"},
		{"Channels' extension", "ChannelsApp/7.0.2 (AppleTV11,1; tvOS 26.3; com.getchannels.dvr 2.23.1910; en-US; US/Pacific)",
			"Unknown", "Apple TV"},
		{"curl", "curl/8.7.1", "Unknown", ""},
		{"macOS Finder", "WebDAVFS/3.0.0 (03008000) Darwin/22.1.0 (x86_64)", "Unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, device := ParseUserAgent(tt.ua)
			if got := deref(client); got != tt.client {
				t.Errorf("client = %q, want %q", got, tt.client)
			}
			if got := deref(device); got != tt.device {
				t.Errorf("device = %q, want %q", got, tt.device)
			}
		})
	}
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
