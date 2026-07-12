package steamavatar

import "testing"

func TestSteamID64Extraction(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"steam_76561198012345678", "76561198012345678", true},
		{"76561198012345678", "76561198012345678", true},
		{"STEAM_76561198012345678", "76561198012345678", true},
		{"", "", false},
		{"steam_", "", false},
		{"xbox_abcdef", "", false},       // console crossplay identity
		{"76561198012345", "", false},    // too short
		{"12345678901234567", "", false}, // 17 digits but wrong range
		{"steam_notanumber1", "", false},
	}
	for _, tc := range tests {
		got, ok := SteamID64(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Errorf("SteamID64(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestImageRejectsNonSteamIdentityWithoutNetwork(t *testing.T) {
	// A console identity must short-circuit before any network call.
	r := New("")
	if _, _, ok := r.Image(nil, "psn_someplayer"); ok {
		t.Fatal("expected non-Steam identity to resolve to no avatar")
	}
}
