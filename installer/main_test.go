package main

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"testing"
)

func TestSafeRelative(t *testing.T) {
	cases := []struct{name string; ok bool}{
		{"mods/example.jar", true},
		{"config/options.txt", true},
		{"../outside.txt", false},
		{"mods/../../outside.jar", false},
		{"/absolute/path", false},
		{"C:\\Windows\\file.txt", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := safeRelative(tc.name)
			if (err == nil) != tc.ok { t.Fatalf("safeRelative(%q) err=%v, want ok=%v", tc.name, err, tc.ok) }
		})
	}
}

func TestVerifyHashValues(t *testing.T) {
	data := []byte("reys-retreat-test")
	sum := sha1.Sum(data)
	valid := map[string]string{"sha1": hex.EncodeToString(sum[:])}
	if err := verifyHashValues(valid, hashOf(data), hash512Of(data)); err != nil { t.Fatalf("valid hash rejected: %v", err) }
	valid["sha1"] = "0000000000000000000000000000000000000000"
	if err := verifyHashValues(valid, hashOf(data), hash512Of(data)); err == nil { t.Fatal("mismatched hash accepted") }
	if err := verifyHashValues(nil, hashOf(data), hash512Of(data)); err == nil { t.Fatal("missing hashes accepted") }
}

func hashOf(data []byte) hash.Hash { h := sha1.New(); _, _ = h.Write(data); return h }
func hash512Of(data []byte) hash.Hash { h := sha512.New(); _, _ = h.Write(data); return h }

func TestIsNvidiumEntry(t *testing.T) {
	cases := []struct {
		name string
		item mrpackFile
		want bool
	}{
		{name: "ordinary mod", item: mrpackFile{Path: "mods/sodium.jar", Downloads: []string{"https://cdn.modrinth.com/sodium.jar"}}, want: false},
		{name: "Nvidium path", item: mrpackFile{Path: "mods/nvidium-0.4.1.jar"}, want: true},
		{name: "Nvidium download URL", item: mrpackFile{Path: "mods/render-optimizations.jar", Downloads: []string{"https://example.com/Nvidium.jar"}}, want: true},
		{name: "Nvidium metadata", item: mrpackFile{Path: "mods/nvidium.pw.toml"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNvidiumEntry(tc.item); got != tc.want {
				t.Fatalf("isNvidiumEntry() = %v, want %v", got, tc.want)
			}
		})
	}
}
