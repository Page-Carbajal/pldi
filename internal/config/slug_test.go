package config

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Trinity AC", "trinity-ac"},
		{"Hello_World", "hello-world"},
		{"--foo--", "foo"},
		{"@@@", ""},
		{"Gelinas HVAC", "gelinas-hvac"},
		{"a  b", "a-b"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

func TestDefaultPrefix(t *testing.T) {
	got := DefaultPrefix("My Site", "i-abc", "2026-01-02-150405")
	want := "my-site/2026-01-02-150405"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = DefaultPrefix("@@@", "i-abc", "2026-01-02-150405")
	want = "i-abc/2026-01-02-150405"
	if got != want {
		t.Fatalf("fallback got %q want %q", got, want)
	}
}
