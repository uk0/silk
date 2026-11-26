package pdfexport

import (
	"bytes"
	"compress/zlib"
	"image"
	"math"
)

// Compress one scanline at a time rather than allocating both complete
// uncompressed color and alpha planes. Opaque Cairo snapshots need no
// alpha encoder. Binary transparency still carries a soft mask.
func encodePixmapToFlatedRGBAndSMask(img image.Image, w, h int) ([]byte, []byte, bool) {
	if img == nil || w <= 0 || h <= 0 || w > math.MaxInt/3 {
		return nil, nil, false
	}
	bounds := img.Bounds()
	if bounds.Dx() != w || bounds.Dy() != h {
		return nil, nil, false
	}
	var opaque bool
	switch v := img.(type) {
	case *image.RGBA:
		opaque = v.Opaque()
	case *image.NRGBA:
		opaque = v.Opaque()
	}
	var rgbBuf, alphaBuf bytes.Buffer
	rgb := zlib.NewWriter(&rgbBuf)
	var mask *zlib.Writer
	var coverage []byte
	if !opaque {
		mask = zlib.NewWriter(&alphaBuf)
		coverage = make([]byte, w)
	}
	ink := make([]byte, w*3)
	hasAlpha := false
	var writeErr error
	for y := 0; y < h && writeErr == nil; y++ {
		rowAlpha := rasterScanline(img, bounds.Min, y, ink, coverage)
		hasAlpha = hasAlpha || rowAlpha
		_, writeErr = rgb.Write(ink)
		if writeErr == nil && mask != nil {
			_, writeErr = mask.Write(coverage)
		}
	}
	err := rgb.Close()
	if mask != nil {
		maskErr := mask.Close()
		if err == nil {
			err = maskErr
		}
	}
	if writeErr != nil || err != nil {
		return nil, nil, false
	}
	if !hasAlpha {
		return rgbBuf.Bytes(), nil, true
	}
	return rgbBuf.Bytes(), alphaBuf.Bytes(), true
}

// Returns true if at least one output alpha byte is not fully opaque.
func rasterScanline(img image.Image, origin image.Point, y int, rgb, alpha []byte) bool {
	hasAlpha := false
	w := len(rgb) / 3
	switch v := img.(type) {
	case *image.RGBA:
		row := v.Pix[y*v.Stride : y*v.Stride+4*w]
		if alpha == nil {
			for x := range w {
				copy(rgb[3*x:3*x+3], row[4*x:4*x+3])
			}
			return false
		}
		for x := range w {
			p := row[4*x : 4*x+4]
			a := uint32(p[3])
			alpha[x] = p[3]
			hasAlpha = hasAlpha || a != 255
			putPremultipliedRGB(rgb[3*x:3*x+3], uint32(p[0]), uint32(p[1]), uint32(p[2]), a, 255)
		}
	case *image.NRGBA:
		row := v.Pix[y*v.Stride : y*v.Stride+4*w]
		for x := range w {
			p := row[4*x : 4*x+4]
			out := rgb[3*x : 3*x+3]
			if p[3] == 0 {
				out[0], out[1], out[2] = 255, 255, 255
			} else {
				copy(out, p[:3])
			}
			if alpha != nil {
				alpha[x] = p[3]
				hasAlpha = hasAlpha || p[3] != 255
			}
		}
	default:
		for x := range w {
			r, g, b, a := img.At(origin.X+x, origin.Y+y).RGBA()
			alpha[x] = byte(a >> 8)
			hasAlpha = hasAlpha || alpha[x] != 255
			putPremultipliedRGB(rgb[3*x:3*x+3], r, g, b, a, 65535)
		}
	}
	return hasAlpha
}

func putPremultipliedRGB(dst []byte, r, g, b, a, maxAlpha uint32) {
	switch a {
	case 0:
		// Invisible RGB only; the soft mask keeps this pixel transparent.
		dst[0], dst[1], dst[2] = 255, 255, 255
	case maxAlpha:
		if maxAlpha == 65535 {
			dst[0], dst[1], dst[2] = byte(r>>8), byte(g>>8), byte(b>>8)
		} else {
			dst[0], dst[1], dst[2] = byte(r), byte(g), byte(b)
		}
	default:
		dst[0] = byte(min(255, int((r*255+a/2)/a)))
		dst[1] = byte(min(255, int((g*255+a/2)/a)))
		dst[2] = byte(min(255, int((b*255+a/2)/a)))
	}
}
