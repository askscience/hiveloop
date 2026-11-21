package main

import (
	"strings"
	"testing"
	"time"
)

func TestMapsURL(t *testing.T) {
	if got := mapsURL("Central Park New York"); got != "https://www.google.com/maps/search/?api=1&query=Central%20Park%20New%20York" {
		t.Fatal(got)
	}
	if got := mapsURL(" https://maps.app.goo.gl/abc123 "); got != "https://maps.app.goo.gl/abc123" {
		t.Fatal(got)
	}
	if got := mapsURL("Via Roma 1 & 2, Lucca"); !strings.Contains(got, "Via%20Roma%201%20%26%202%2C%20Lucca") {
		t.Fatal(got)
	}
	var b strings.Builder
	v := (&App{cfg: &Config{TZ: time.UTC}}).pubEventView(pubIT, Event{Title: "x", Start: "2026-11-28T07:00:00Z", End: "2026-11-28T10:00:00Z", Location: "https://maps.app.goo.gl/abc123"}, "")
	pubTpl.Execute(&b, pubPage{T: pubIT, Events: []pubEvent{v}})
	if strings.Contains(b.String(), "goo.gl/abc123 ·") || !strings.Contains(b.String(), `href="https://maps.app.goo.gl/abc123"`) {
		t.Fatal("pasted map link not rendered as link only")
	}
}
