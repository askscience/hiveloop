package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestReadWebpageBlocksInternal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, u := range []string{"http://127.0.0.1:11434/api/tags", "http://localhost:8090/", "http://192.168.1.1/", "http://100.64.0.1/", "file:///etc/passwd"} {
		if _, _, err := readWebpage(ctx, u); err == nil {
			t.Errorf("%s should be blocked", u)
		}
	}
}

func TestWebSearchLive(t *testing.T) {
	if os.Getenv("HIVELOOP_NET_TESTS") == "" {
		t.Skip("set HIVELOOP_NET_TESTS=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := webSearch(ctx, "Notte di San Lorenzo stelle cadenti", "it-it", 5)
	if err != nil || len(res) == 0 {
		t.Fatalf("search: %v %d", err, len(res))
	}
	t.Logf("%+v", res[0])
	title, text, err := readWebpage(ctx, res[0].URL)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	t.Logf("title=%q len=%d", title, len(text))
}

func TestSearchFallbackLive(t *testing.T) {
	if os.Getenv("HIVELOOP_NET_TESTS") == "" {
		t.Skip("set HIVELOOP_NET_TESTS=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	res, engine, err := searchWeb(ctx, "picco Perseidi 2027", "it-it", 5)
	if err != nil || len(res) == 0 {
		t.Fatalf("search: %v", err)
	}
	t.Logf("engine=%s first=%s %s", engine, res[0].Title, res[0].URL)
	b, err := bingSearch(ctx, "Perseidi 2027", "it", 5)
	if err != nil || len(b) == 0 {
		t.Fatalf("bing: %v %d", err, len(b))
	}
	t.Logf("bing first: %s %s", b[0].Title, b[0].URL)
}
