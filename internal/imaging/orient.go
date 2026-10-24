package imaging

import (
	"image"
	"image/draw"
)

// Orient applies an EXIF orientation (2–8) to img and returns the corrected
// image. Orientation 1, or anything invalid, returns img unchanged.
func Orient(img image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := newOrientedImage(img, dw, dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := sourceCoords(orientation, w, h, x, y)
			dst.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// sourceCoords maps an output coordinate to the source pixel that belongs
// there for the given EXIF orientation.
func sourceCoords(orientation, w, h, x, y int) (sx, sy int) {
	switch orientation {
	case 2:
		return w - 1 - x, y
	case 3:
		return w - 1 - x, h - 1 - y
	case 4:
		return x, h - 1 - y
	case 5:
		return y, x
	case 6:
		return y, h - 1 - x
	case 7:
		return w - 1 - y, h - 1 - x
	case 8:
		return w - 1 - y, x
	default:
		return x, y
	}
}

// newOrientedImage keeps 16-bit depth when the source has it and otherwise
// works in 8-bit non-premultiplied RGBA.
func newOrientedImage(img image.Image, w, h int) draw.Image {
	rect := image.Rect(0, 0, w, h)
	switch img.(type) {
	case *image.NRGBA64, *image.RGBA64:
		return image.NewNRGBA64(rect)
	case *image.Gray16:
		return image.NewGray16(rect)
	case *image.Gray:
		return image.NewGray(rect)
	default:
		return image.NewNRGBA(rect)
	}
}
