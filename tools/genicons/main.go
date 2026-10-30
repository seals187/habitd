//go:build ignore

// Generates the PWA icons: a green progress ring with a check mark.
// Run from the repository root: go run tools/genicons/main.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

var (
	bg    = color.RGBA{0x12, 0x14, 0x12, 0xff}
	track = color.RGBA{0x2e, 0x32, 0x2f, 0xff}
	green = color.RGBA{0x4f, 0xb2, 0x83, 0xff}
)

func mix(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 0xff}
}

func clamp(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func segment(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := clamp(((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy))
	return math.Hypot(px-ax-t*dx, py-ay-t*dy)
}

func render(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	for y := range size {
		for x := range size {
			px, py := (float64(x)+.5)/s, (float64(y)+.5)/s
			c := bg
			r := math.Hypot(px-.5, py-.5)
			ring := 1 - clamp((math.Abs(r-.3)-.035)*s)
			angle := math.Mod(math.Atan2(py-.5, px-.5)+math.Pi/2+2*math.Pi, 2*math.Pi)
			if angle < 2*math.Pi*.75 {
				c = mix(c, green, ring)
			} else {
				c = mix(c, track, ring)
			}
			check := math.Min(segment(px, py, .39, .51, .47, .59), segment(px, py, .47, .59, .62, .42))
			c = mix(c, green, 1-clamp((check-.03)*s))
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func main() {
	for name, size := range map[string]int{"web/static/icons/icon-192.png": 192, "web/static/icons/icon-512.png": 512} {
		f, err := os.Create(name)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, render(size)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
