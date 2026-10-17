package main

import "testing"

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Living Room PC":    "living-room-pc",
		"  --Studio__Mac--": "studio-mac",
		"Café Box":          "caf-box",
		"!!!":               "",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
