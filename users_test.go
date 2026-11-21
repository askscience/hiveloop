package main

import "testing"

func TestNormalizeUsername(t *testing.T) {
	for in, want := range map[string]string{"Mario Rossi": "mario.rossi", " maria.rossì ": "maria.rossi", "Élodie  D'Arco": "elodie.darco", "lu": "lu"} {
		if got := normalizeUsername(in); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
}
