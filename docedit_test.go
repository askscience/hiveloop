package main

import "testing"

func TestMdName(t *testing.T) {
	for in, want := range map[string]string{
		"Verbale 16.03 riunione": "Verbale 16.03 riunione.md",
		"note.md":                "note.md",
		"Piano.MD":               "Piano.md",
		"guida.markdown":         "guida.md",
		"  ":                     ".md",
		"a/b:c":                  "b_c.md",
	} {
		if got := mdName(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
