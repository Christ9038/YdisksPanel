package imgproc

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func mk(w, h int) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, color.NRGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	return im
}

func enc(t *testing.T, f func(*bytes.Buffer) error) []byte {
	var b bytes.Buffer
	if err := f(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func dims(t *testing.T, data []byte) (int, int, string) {
	c, f, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return c.Width, c.Height, f
}

func TestIconAlwaysSquareAndAtMost256(t *testing.T) {
	for _, d := range [][2]int{{1000, 500}, {300, 900}, {4000, 4000}, {512, 512}} {
		src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(d[0], d[1])) })
		r, err := Process(bytes.NewReader(src), 5<<20, Icon, nil)
		if err != nil {
			t.Fatal(err)
		}
		w, h, f := dims(t, r.Data)
		if w != h || w > 256 || f != "png" || r.Ext != "png" {
			t.Fatalf("%v -> %dx%d %s", d, w, h, f)
		}
	}
}

func TestDefaultCropIsCentred(t *testing.T) {
	// 300x100 image: left third red, middle blue, right third green. Centre crop = the blue square.
	im := image.NewNRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 300; x++ {
			c := color.NRGBA{0, 0, 255, 255}
			if x < 100 {
				c = color.NRGBA{255, 0, 0, 255}
			} else if x >= 200 {
				c = color.NRGBA{0, 255, 0, 255}
			}
			im.Set(x, y, c)
		}
	}
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, im) })
	r, err := Process(bytes.NewReader(src), 1<<20, Icon, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := png.Decode(bytes.NewReader(r.Data))
	if w, h, _ := dims(t, r.Data); w != 100 || h != 100 {
		t.Fatalf("size %dx%d", w, h)
	}
	for _, p := range []image.Point{{1, 50}, {50, 50}, {98, 50}} {
		if rr, gg, bb, _ := out.At(p.X, p.Y).RGBA(); rr != 0 || gg != 0 || bb == 0 {
			t.Fatalf("pixel %v not blue", p)
		}
	}
}

func TestExplicitCropSelectsRegion(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 300, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 300; x++ {
			c := color.NRGBA{0, 0, 255, 255}
			if x < 100 {
				c = color.NRGBA{255, 0, 0, 255}
			}
			im.Set(x, y, c)
		}
	}
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, im) })
	r, err := Process(bytes.NewReader(src), 1<<20, Icon, &Crop{X: 0, Y: 0, Size: 100})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := png.Decode(bytes.NewReader(r.Data))
	if rr, _, bb, _ := out.At(50, 50).RGBA(); rr == 0 || bb != 0 {
		t.Fatal("expected the red region")
	}
}

func TestCropBeyondImageIsTransparentPadding(t *testing.T) {
	// Fit a whole 200x100 picture inside a 200x200 square: top and bottom bands are transparent.
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(200, 100)) })
	r, err := Process(bytes.NewReader(src), 1<<20, Icon, &Crop{X: 0, Y: -50, Size: 200})
	if err != nil {
		t.Fatal(err)
	}
	if w, h, _ := dims(t, r.Data); w != 200 || h != 200 {
		t.Fatalf("size %dx%d", w, h)
	}
	out, _ := png.Decode(bytes.NewReader(r.Data))
	if _, _, _, a := out.At(100, 10).RGBA(); a != 0 {
		t.Fatal("top band must be transparent")
	}
	if _, _, _, a := out.At(100, 100).RGBA(); a == 0 {
		t.Fatal("centre must be opaque")
	}
	// a 400px square holding the picture is downscaled to 256, picture still centred
	r, err = Process(bytes.NewReader(src), 1<<20, Icon, &Crop{X: -100, Y: -150, Size: 400})
	if err != nil {
		t.Fatal(err)
	}
	out, _ = png.Decode(bytes.NewReader(r.Data))
	if w, _, _ := dims(t, r.Data); w != 256 {
		t.Fatalf("expected 256, got %d", w)
	}
	if _, _, _, a := out.At(128, 128).RGBA(); a == 0 {
		t.Fatal("centre must be opaque")
	}
}

func TestBadCropRejected(t *testing.T) {
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(100, 100)) })
	for _, c := range []Crop{{0, 0, 4}, {0, 0, 5000}, {-500, 0, 100}, {0, 500, 100}, {200, 0, 100}} {
		if _, err := Process(bytes.NewReader(src), 1<<20, Icon, &c); err != ErrCrop {
			t.Errorf("%+v: want ErrCrop, got %v", c, err)
		}
	}
}

func TestSmallNotUpscaled(t *testing.T) {
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(64, 48)) })
	r, err := Process(bytes.NewReader(src), 1<<20, Icon, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w, h, _ := dims(t, r.Data); w != 48 || h != 48 {
		t.Fatalf("must not upscale (centre-cropped to 48): %dx%d", w, h)
	}
}

func TestJPEGBecomesPNGAndTransparencyKept(t *testing.T) {
	src := enc(t, func(b *bytes.Buffer) error { return jpeg.Encode(b, mk(400, 400), nil) })
	r, err := Process(bytes.NewReader(src), 5<<20, Icon, nil)
	if err != nil || r.Ext != "png" || r.ContentType != "image/png" {
		t.Fatalf("%v %+v", err, r)
	}
	tr := image.NewNRGBA(image.Rect(0, 0, 300, 300)) // fully transparent
	src = enc(t, func(b *bytes.Buffer) error { return png.Encode(b, tr) })
	r, _ = Process(bytes.NewReader(src), 5<<20, Icon, nil)
	out, _ := png.Decode(bytes.NewReader(r.Data))
	if _, _, _, a := out.At(10, 10).RGBA(); a != 0 {
		t.Fatal("alpha lost")
	}
}

func TestGIFFirstFrame(t *testing.T) {
	g := gif.GIF{}
	for i := 0; i < 3; i++ {
		p := image.NewPaletted(image.Rect(0, 0, 10, 10), color.Palette{color.Black, color.White})
		g.Image, g.Delay = append(g.Image, p), append(g.Delay, 5)
	}
	src := enc(t, func(b *bytes.Buffer) error { return gif.EncodeAll(b, &g) })
	r, err := Process(bytes.NewReader(src), 1<<20, Icon, nil)
	if err != nil || r.Ext != "png" {
		t.Fatalf("%v", err)
	}
}

func TestWallpaperJPEG(t *testing.T) {
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(2400, 1200)) })
	r, err := Process(bytes.NewReader(src), 12<<20, Wallpaper, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w, h, f := dims(t, r.Data); w != 1920 || h != 960 || f != "jpeg" || r.Ext != "jpg" {
		t.Fatalf("got %dx%d %s", w, h, f)
	}
}

func TestRejects(t *testing.T) {
	for name, data := range map[string]string{
		"svg":   `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"html":  `<html><script>alert(1)</script></html>`,
		"empty": ``,
		"text":  strings.Repeat("hello", 100),
	} {
		if _, err := Process(strings.NewReader(data), 1<<20, Icon, nil); err != ErrFormat {
			t.Errorf("%s: want ErrFormat, got %v", name, err)
		}
	}
	// truncated PNG: valid header, broken body
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(200, 200)) })
	if _, err := Process(bytes.NewReader(src[:len(src)/2]), 1<<20, Icon, nil); err == nil {
		t.Error("truncated image must fail")
	}
	// byte limit
	if _, err := Process(bytes.NewReader(src), 100, Icon, nil); err != ErrTooLarge {
		t.Errorf("limit: %v", err)
	}
}

func TestPixelBombRejectedBeforeDecode(t *testing.T) {
	oldSide, oldPx := maxSide, maxPixels
	maxSide, maxPixels = 500, 100_000
	defer func() { maxSide, maxPixels = oldSide, oldPx }()
	src := enc(t, func(b *bytes.Buffer) error { return png.Encode(b, mk(400, 400)) }) // 160k px > limit
	if _, err := Process(bytes.NewReader(src), 5<<20, Icon, nil); err != ErrTooLarge {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func TestMetadataStripped(t *testing.T) {
	// A JPEG with an EXIF-like APP1 segment: after re-encode the marker must be gone.
	src := enc(t, func(b *bytes.Buffer) error { return jpeg.Encode(b, mk(300, 300), nil) })
	app1 := []byte{0xFF, 0xE1, 0x00, 0x10, 'E', 'x', 'i', 'f', 0, 0, 'G', 'P', 'S', 'D', 'A', 'T', 'A', 0}
	tainted := append(append(append([]byte{}, src[:2]...), app1...), src[2:]...)
	r, err := Process(bytes.NewReader(tainted), 1<<20, Icon, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(r.Data, []byte("GPSDATA")) || bytes.Contains(r.Data, []byte("Exif")) {
		t.Fatal("metadata survived re-encoding")
	}
}
