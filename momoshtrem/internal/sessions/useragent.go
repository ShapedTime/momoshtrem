package sessions

import "strings"

// Clients a session's User-Agent can name.
const (
	ClientInfuse  = "Infuse"
	ClientVLC     = "VLC"
	ClientSafari  = "Safari"
	ClientKodi    = "Kodi"
	ClientUnknown = "Unknown"
)

// ParseUserAgent names the client and the device in a User-Agent:
//   - client: Infuse (Infuse, Infuse-Direct, Infuse-Library, …), VLC,
//     Safari, Kodi, else "Unknown"; nil without a User-Agent (Infuse 8
//     sends none over WebDAV at times);
//   - device: Apple TV, iPad, iPhone, Mac, Android, Windows or Linux when
//     the User-Agent says so, else nil. Infuse's own ("Infuse-Direct/7.7")
//     usually doesn't.
func ParseUserAgent(ua string) (client, device *string) {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return nil, nil
	}
	lower := strings.ToLower(ua)
	c := clientOf(lower)
	return &c, deviceOf(lower)
}

func clientOf(ua string) string {
	switch {
	case strings.HasPrefix(ua, "infuse"):
		return ClientInfuse
	case strings.Contains(ua, "vlc"):
		return ClientVLC // VLC/3.0.20 LibVLC/3.0.20, VLC-TV/102.1 CFNetwork/…
	case strings.Contains(ua, "kodi/") || strings.Contains(ua, "xbmc/"):
		return ClientKodi
	case strings.Contains(ua, "safari/") && !containsAny(ua, "chrome/", "chromium/", "crios/", "edg/", "edgios/", "fxios/", "opr/", "android"):
		return ClientSafari // Chrome and the rest say Safari too
	default:
		return ClientUnknown
	}
}

func deviceOf(ua string) *string {
	var d string
	switch {
	case containsAny(ua, "appletv", "apple tv", "tvos", "vlc-tv/"):
		d = "Apple TV"
	case strings.Contains(ua, "ipad"):
		d = "iPad"
	case strings.Contains(ua, "iphone"):
		d = "iPhone" // before Mac: iOS says "like Mac OS X"
	case containsAny(ua, "macintosh", "mac os x", "macos"):
		d = "Mac"
	case strings.Contains(ua, "android"):
		d = "Android"
	case strings.Contains(ua, "windows"):
		d = "Windows"
	case containsAny(ua, "linux", "x11"):
		d = "Linux"
	default:
		return nil
	}
	return &d
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
