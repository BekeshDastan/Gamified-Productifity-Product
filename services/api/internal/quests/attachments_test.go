package quests

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestFileSignaturesDecodeAndPixelLimit(t *testing.T) {
	small := image.NewRGBA(image.Rect(0, 0, 2, 2))
	small.Set(0, 0, color.Black)
	for _, kind := range []string{"png", "jpg"} {
		var b bytes.Buffer
		var err error
		if kind == "png" {
			err = png.Encode(&b, small)
		} else {
			err = jpeg.Encode(&b, small, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := validateFile("picture."+kind, b.Bytes()); err != nil {
			t.Fatal("valid image rejected", err)
		}
		if _, err := validateFile("picture."+kind, b.Bytes()[:20]); err == nil {
			t.Fatal("truncated image accepted")
		}
	}
	var huge bytes.Buffer
	if err := png.Encode(&huge, image.NewRGBA(image.Rect(0, 0, 8193, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := validateFile("too-wide.png", huge.Bytes()); err == nil {
		t.Fatal("pixel dimensions ignored")
	}
	for _, name := range []string{"fake.png", "../x.txt", `..\x.txt`, "x.svg", "x.exe"} {
		if _, err := validateFile(name, []byte("<html>hi</html>")); err == nil {
			t.Fatal("unsafe type/path")
		}
	}
	if _, err := validateFile("x.txt", []byte{0, 1, 2}); err == nil {
		t.Fatal("binary text")
	}
	if _, err := validateFile("x.txt", bytes.Repeat([]byte("a"), maxFile+1)); err == nil {
		t.Fatal("size ignored")
	}
}
