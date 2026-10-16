package torrent

import "testing"

func TestNormalizeInfoHash(t *testing.T) {
	const hex = "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercase hex", hex, hex},
		{"uppercase hex", "DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C", hex},
		{"base32", "3WBFL3G4PSSV7MF37AJSHWDQMLNR63I4", hex},
		{"lowercase base32", "3wbfl3g4pssv7mf37ajshwdqmlnr63i4", hex},
		{"empty", "", ""},
		{"too short", hex[:39], ""},
		{"not hex", "zz8255ecdc7ca55fb0bbf81323d87062db1f6d1c", ""},
		{"not base32", "1WBFL3G4PSSV7MF37AJSHWDQMLNR63I4", ""},
		{"path", "../../../../../../../../../../etc/passwd", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeInfoHash(tt.in); got != tt.want {
				t.Errorf("NormalizeInfoHash(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
