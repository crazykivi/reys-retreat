package main

import (
	"strings"
	"testing"
)

func TestSafeSlug(t *testing.T) {
	cases := map[string]string{
		"AppleSkin":       "appleskin",
		"Mod: Nice Name!": "mod-nice-name",
		"  --Hi There-- ": "hi-there",
		"":                "",
	}
	for input, want := range cases {
		if got := safeSlug(input); got != want {
			t.Errorf("safeSlug(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMakePackwizMeta(t *testing.T) {
	project := modrinthProject{ID: "abc123", Title: "Example Mod", ClientSide: "required", ServerSide: "unsupported"}
	version := modrinthVersion{ID: "ver123", ProjectID: "abc123", VersionNum: "1.2.3"}
	file := modrinthFile{Filename: "example.jar", URL: "https://cdn.modrinth.com/data/abc123/versions/ver123/example.jar", Hashes: map[string]string{"sha512": strings.Repeat("a", 128)}}
	got, err := makePackwizMeta(project, version, file, "renamed-example.jar")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name = "Example Mod"`,
		`filename = "renamed-example.jar"`,
		`side = "client"`,
		`hash-format = "sha512"`,
		`mod-id = "abc123"`,
		`version = "ver123"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("metadata does not contain %q:\n%s", want, got)
		}
	}
}
