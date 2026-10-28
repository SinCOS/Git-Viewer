package deeplink

import (
	"strings"
	"testing"
)

func TestRoundTripPreservesRemotePaths(t *testing.T) {
	want := Request{
		Host: "dev-box", Path: "/tmp/中文 folder/\nquote'file.go", Kind: File,
		NewWindow: true, Line: 12, Column: 4, DiffPath: "/tmp/另一 个.go",
	}
	raw, err := Build(want)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(raw, "\n\"' ") {
		t.Fatalf("link is not shell-safe: %q", raw)
	}
	got, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestRejectsUnsafeOrMalformedLinks(t *testing.T) {
	cases := []string{
		"gitviewer://open?host=bad%20host&path64=L3RtcA&kind=folder",
		"gitviewer://open?host=dev&path64=relative&kind=folder",
		"gitviewer://open?host=dev&path64=L3RtcA&kind=unknown",
		"gitviewer://open?host=dev&path64=L3RtcA&kind=folder&extra=1",
		"gitviewer://open?host=dev&path64=***&kind=folder",
		"https://open?host=dev&path64=L3RtcA&kind=folder",
	}
	for _, raw := range cases {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
