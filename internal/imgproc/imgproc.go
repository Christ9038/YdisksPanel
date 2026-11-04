// Package imgproc validates, downsizes and re-encodes user-uploaded images.
//
// Every upload is fully decoded and re-encoded, so the stored bytes are always
// a clean image produced by this process: no EXIF/GPS metadata, no embedded
// payloads, no animation, and never larger than the configured bounds.
package imgproc

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register WebP decoder
)

type Kind int

const (
	Icon      Kind = iota // <=256px, PNG (keeps transparency)
	Wallpaper             // <=1920px, JPEG
)

const (
	IconMax      = 256
	WallpaperMax = 1920
)

// Crop selects a square region of the *source* image, in source pixels. The
// region may extend beyond the image (the excess becomes transparent), which
// lets a user fit an entire non-square picture inside the square.
type Crop struct{ X, Y, Size int }

var (
	ErrCrop     = errors.New("invalid crop area")
	ErrFormat   = errors.New("only PNG, JPEG, WebP or GIF images are allowed")
	ErrTooLarge = errors.New("image dimensions are too large")
	ErrDecode   = errors.New("image could not be decoded")
)

// Declared-size guards, checked from the header *before* decoding pixels so a
// tiny file claiming 60000x60000 can't exhaust memory. Variables so tests can lower them.
var (
	maxSide   = 10000
	maxPixels = 40_000_000
)

type Result struct {
	Data        []byte
	Ext         string // "png" | "jpg"
	ContentType string
	Width       int
	Height      int
}

// Process reads an image from r (at most limit bytes) and returns the normalized result.
//
// Icons are always made square: crop selects the region, and when nil the
// largest centred square is used. Wallpapers keep their aspect ratio.
func Process(r io.Reader, limit int64, kind Kind, crop *Crop) (*Result, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, ErrTooLarge
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrFormat
	}
	switch format {
	case "png", "jpeg", "gif", "webp":
	default:
		return nil, ErrFormat
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return nil, ErrTooLarge
	}

	var src image.Image
	if format == "gif" {
		// Animated GIFs collapse to their first frame.
		g, err := gif.Decode(bytes.NewReader(raw))
		if err != nil {
			return nil, ErrDecode
		}
		src = g
	} else if src, _, err = image.Decode(bytes.NewReader(raw)); err != nil {
		return nil, ErrDecode
	}

	bound := IconMax
	if kind == Wallpaper {
		bound = WallpaperMax
		src = fit(src, bound)
	} else {
		src, err = squareCrop(src, crop, bound)
		if err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	res := &Result{Width: src.Bounds().Dx(), Height: src.Bounds().Dy()}
	if kind == Wallpaper {
		if err := jpeg.Encode(&buf, flatten(src), &jpeg.Options{Quality: 82}); err != nil {
			return nil, err
		}
		res.Ext, res.ContentType = "jpg", "image/jpeg"
	} else {
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(&buf, src); err != nil {
			return nil, err
		}
		res.Ext, res.ContentType = "png", "image/png"
	}
	res.Data = buf.Bytes()
	return res, nil
}

// squareCrop returns the cropped region as a square image of at most bound px per side.
func squareCrop(src image.Image, c *Crop, bound int) (image.Image, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if c == nil {
		side := min(w, h)
		c = &Crop{X: (w - side) / 2, Y: (h - side) / 2, Size: side}
	}
	// Bounded so a hostile request can't ask for huge allocations or an empty overlap.
	if c.Size < 8 || c.Size > 2*max(w, h) || c.X >= w || c.Y >= h || c.X+c.Size <= 0 || c.Y+c.Size <= 0 {
		return nil, ErrCrop
	}
	t := min(c.Size, bound) // never upscale
	f := float64(t) / float64(c.Size)

	dst := image.NewNRGBA(image.Rect(0, 0, t, t)) // transparent
	inter := image.Rect(c.X, c.Y, c.X+c.Size, c.Y+c.Size).Intersect(image.Rect(0, 0, w, h)).Add(b.Min)
	rel := func(v, origin int) int { return int(math.Round(float64(v-origin) * f)) }
	dr := image.Rect(rel(inter.Min.X-b.Min.X, c.X), rel(inter.Min.Y-b.Min.Y, c.Y), rel(inter.Max.X-b.Min.X, c.X), rel(inter.Max.Y-b.Min.Y, c.Y)).Intersect(dst.Bounds())
	if dr.Empty() {
		return nil, ErrCrop
	}
	draw.CatmullRom.Scale(dst, dr, src, inter, draw.Src, nil)
	return dst, nil
}

// fit scales src down (never up) so neither side exceeds bound, preserving aspect ratio.
func fit(src image.Image, bound int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= bound && h <= bound {
		return src
	}
	nw, nh := bound, bound
	if w >= h {
		nh = max(1, h*bound/w)
	} else {
		nw = max(1, w*bound/h)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// flatten composites onto white (JPEG has no alpha).
func flatten(src image.Image) image.Image {
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Over)
	return dst
}
