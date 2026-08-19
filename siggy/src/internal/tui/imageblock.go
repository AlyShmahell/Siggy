package tui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/image/draw"
)

func imagePreview(abs string, cols, maxRows int) (string, int, int) {
	if abs == "" || cols < 1 {
		return "", 0, 0
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", 0, 0
	}
	defer f.Close()
	img, _, err := image.Decode(io.LimitReader(f, maxImageBytes+1))
	if err != nil || img == nil {
		return "", 0, 0
	}
	return halfBlockFit(img, cols, maxRows)
}

func halfBlockFit(img image.Image, cols, maxRows int) (string, int, int) {
	if img == nil {
		return "", 0, 0
	}
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW < 1 || srcH < 1 {
		return "", 0, 0
	}
	if cols < 1 {
		cols = 1
	}
	if maxRows < 1 {
		maxRows = 40
	}
	rowPairs := (srcH*cols + srcW) / (srcW * 2)
	if rowPairs < 1 {
		rowPairs = 1
	}
	if rowPairs > maxRows {
		rowPairs = maxRows
		cols = (srcW*rowPairs*2 + srcH/2) / srcH
		if cols < 1 {
			cols = 1
		}
	}
	return halfBlock(img, cols, rowPairs), cols, rowPairs
}

func halfBlock(img image.Image, cols, rowPairs int) string {
	if img == nil || cols < 1 || rowPairs < 1 {
		return ""
	}
	src := img.Bounds()
	if src.Dx() < 1 || src.Dy() < 1 {
		return ""
	}
	dstH := rowPairs * 2
	dst := image.NewNRGBA(image.Rect(0, 0, cols, dstH))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{matteColor()}, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, src, draw.Over, nil)
	var out strings.Builder
	for y := 0; y < rowPairs; y++ {
		if y > 0 {
			out.WriteByte('\n')
		}
		for x := 0; x < cols; x++ {
			st := lipgloss.NewStyle().
				Foreground(lipgloss.Color(pixelHex(dst.At(x, 2*y)))).
				Background(lipgloss.Color(pixelHex(dst.At(x, 2*y+1))))
			out.WriteString(st.Render("▀"))
		}
	}
	return out.String()
}

func matteColor() color.NRGBA {
	s := strings.TrimPrefix(string(colBg), "#")
	if len(s) != 6 {
		return color.NRGBA{R: 0x0e, G: 0x0f, B: 0x11, A: 255}
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{R: 0x0e, G: 0x0f, B: 0x11, A: 255}
	}
	return color.NRGBA{
		R: uint8(n >> 16),
		G: uint8(n >> 8),
		B: uint8(n),
		A: 255,
	}
}

func pixelHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
