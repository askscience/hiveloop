package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestOptimizeImage(t *testing.T) {
	// a 3000x2000 "photo" (gradient + noise) keeps its shape, longest side 1080
	img := image.NewRGBA(image.Rect(0, 0, 3000, 2000))
	for y := 0; y < 2000; y++ {
		for x := 0; x < 3000; x++ {
			img.Set(x, y, color.RGBA{uint8(x / 12), uint8(y / 8), uint8((x * y) % 251), 255})
		}
	}
	var in bytes.Buffer
	jpeg.Encode(&in, img, &jpeg.Options{Quality: 92})
	out, err := optimizeImage(in.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg, f, _ := image.DecodeConfig(bytes.NewReader(out))
	if f != "jpeg" || cfg.Width != 1080 || cfg.Height != 720 {
		t.Fatalf("got %s %dx%d", f, cfg.Width, cfg.Height)
	}
	if len(out) >= in.Len() {
		t.Fatalf("not smaller: %d -> %d", in.Len(), len(out))
	}
	// transparent PNG: accepted, white background, JPEG out
	var p bytes.Buffer
	png.Encode(&p, image.NewNRGBA(image.Rect(0, 0, 400, 300)))
	if out, err := optimizeImage(p.Bytes()); err != nil || !bytes.HasPrefix(out, []byte{0xff, 0xd8}) {
		t.Fatalf("png: %v", err)
	}
	// not an image
	if _, err := optimizeImage([]byte("<svg onload=alert(1)>")); err == nil {
		t.Fatal("svg accepted")
	}
	// real covers, if present (manual check of the gain)
	files, _ := filepath.Glob(os.Getenv("HIVELOOP_TEST_IMAGES") + "/*.jpg")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		o, err := optimizeImage(b)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		c0, _, _ := image.DecodeConfig(bytes.NewReader(b))
		c1, _, _ := image.DecodeConfig(bytes.NewReader(o))
		if d := os.Getenv("HIVELOOP_TEST_OUT"); d != "" {
			os.WriteFile(filepath.Join(d, "opt-"+filepath.Base(f)), o, 0o644)
		}
		t.Logf("%s: %dx%d %d KB -> %dx%d %d KB", filepath.Base(f), c0.Width, c0.Height, len(b)>>10, c1.Width, c1.Height, len(o)>>10)
	}
}
