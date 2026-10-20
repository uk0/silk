//go:build !silk_pure_go

package cairo

import (
	"encoding/binary"
	"errors"
	"image"
	"unsafe"
)

func surfaceImagePixelsToRGBA(pix []byte, opaque bool) {
	for i := 0; i+3 < len(pix); i += 4 {
		p := binary.NativeEndian.Uint32(pix[i:])
		pix[i], pix[i+1], pix[i+2], pix[i+3] = byte(p>>16), byte(p>>8), byte(p), byte(p>>24)
		if opaque {
			pix[i+3] = 255
		}
	}
}

// Copy only the overlapping image, anchored at its Bounds().Min, preserving
// destination pixels outside it. Cairo's native-endian ARGB32 is premultiplied,
// exactly like image.RGBA and Color.RGBA(), not like PNG's image.NRGBA bytes.
// Write straight into the surface: no full-surface readback or scratch image.
func setSurfaceImage(s *Surface, img image.Image) error {
	if img == nil {
		return errors.New("cairo.Surface.SetImage: nil image")
	}
	format := s.Format()
	if format != FORMAT_ARGB32 && format != FORMAT_RGB24 && format != FORMAT_A8 {
		return errors.New("cairo.Surface.SetImage: unsupported format")
	}
	bounds := img.Bounds()
	w, h := min(s.Width(), bounds.Dx()), min(s.Height(), bounds.Dy())
	if w <= 0 || h <= 0 {
		return nil
	}
	s.Flush()
	ptr, stride := s.DataPtr(), s.Stride()
	if ptr == nil {
		return errors.New("cairo.Surface.SetImage: failed to get data")
	}
	dst := unsafe.Slice((*byte)(ptr), stride*s.Height())
	if format == FORMAT_A8 {
		switch src := img.(type) {
		case *image.Alpha:
			for y := 0; y < h; y++ {
				copy(dst[y*stride:y*stride+w], src.Pix[y*src.Stride:y*src.Stride+w])
			}
			s.MarkDirty()
			return nil
		case *image.RGBA:
			copySurfaceAlpha(dst, stride, src.Pix, src.Stride, w, h)
			s.MarkDirty()
			return nil
		case *image.NRGBA:
			copySurfaceAlpha(dst, stride, src.Pix, src.Stride, w, h)
			s.MarkDirty()
			return nil
		}
	}
	if rgba, ok := img.(*image.RGBA); ok && format != FORMAT_A8 {
		// Cairo allocates aligned 32-bit pixels. Store one native word rather
		// than four byte stores per pixel; Go RGBA source bytes are always RGBA
		// regardless of host endianness.
		pixels := unsafe.Slice((*uint32)(ptr), stride*s.Height()/4)
		for y := 0; y < h; y++ {
			src := rgba.Pix[y*rgba.Stride : y*rgba.Stride+w*4]
			row := pixels[y*(stride/4) : y*(stride/4)+w]
			for x := range row {
				p := binary.LittleEndian.Uint32(src[x*4:])
				p = (p & 0xff00ff00) | (p&0xff)<<16 | (p>>16)&0xff
				if format == FORMAT_RGB24 {
					p |= 0xff000000
				}
				row[x] = p
			}
		}
	} else if src, ok := img.(*image.NRGBA); ok && format != FORMAT_A8 {
		// PNGs commonly decode to NRGBA. Avoid At()/Color interface boxing
		// for every pixel, but preserve Go's exact 16-bit premultiplication
		// before quantizing to 8-bit (v*a/255 alone differs at some levels).
		pixels := unsafe.Slice((*uint32)(ptr), stride*s.Height()/4)
		for y := 0; y < h; y++ {
			row := pixels[y*(stride/4) : y*(stride/4)+w]
			data := src.Pix[y*src.Stride : y*src.Stride+w*4]
			for x := range row {
				a := uint32(data[x*4+3])
				r := premultiplyImageByte(uint32(data[x*4]), a)
				g := premultiplyImageByte(uint32(data[x*4+1]), a)
				b := premultiplyImageByte(uint32(data[x*4+2]), a)
				if format == FORMAT_RGB24 {
					a = 255
				}
				row[x] = a<<24 | r<<16 | g<<8 | b
			}
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				if format == FORMAT_A8 {
					dst[y*stride+x] = byte(a >> 8)
					continue
				}
				if format == FORMAT_RGB24 {
					a = 65535
				}
				binary.NativeEndian.PutUint32(dst[y*stride+x*4:], (a>>8)<<24|(r>>8)<<16|(g>>8)<<8|(b>>8))
			}
		}
	}
	s.MarkDirty()
	return nil
}

func premultiplyImageByte(value, alpha uint32) uint32 {
	return value * 0x101 * alpha / (0xff * 0x100)
}

func copySurfaceAlpha(dst []byte, stride int, src []byte, srcStride, w, h int) {
	for y := 0; y < h; y++ {
		row := dst[y*stride : y*stride+w]
		data := src[y*srcStride : y*srcStride+w*4]
		for x := range row {
			row[x] = data[x*4+3]
		}
	}
}
