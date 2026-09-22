// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// The triangular upsampling below is not from the Go standard library. See
// NOTICE for what this fork changes and why.

package jpeg

import (
	"image"
	"image/color"
)

// drawYCbCrFancy writes a YCbCr picture into an RGBA one, upsampling its
// chroma the way libjpeg does rather than by repeating each sample.
//
// image/internal/imageutil.DrawYCbCr, which this replaces, takes the chroma
// sample whose index is the output coordinate divided down -- nearest
// neighbour. libjpeg instead reads between the samples: jdsample.c's
// h2v2_fancy_upsample weights the nearer sample three times the further one in
// each direction, so a chroma edge arrives as a ramp rather than a staircase.
//
// It matters here because this is the only path by which a FOUR-component
// picture reaches a caller. A three-component one comes back as an
// *image.YCbCr with its planes intact and a caller can upsample them itself;
// a four-component one is merged here, so whatever this does is what the
// caller gets. Against poppler, which is libjpeg, repeating the samples costs
// up to 36 levels on a CMYK picture with 2x2 chroma.
func drawYCbCrFancy(dst *image.RGBA, src *image.YCbCr) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	hf, vf := subsampleFactors(src.SubsampleRatio)
	cw, ch := (w+hf-1)/hf, (h+vf-1)/vf
	cb := upsample(src.Cb, src.CStride, cw, ch, w, h, hf, vf)
	cr := upsample(src.Cr, src.CStride, cw, ch, w, h, hf, vf)
	for y := range h {
		for x := range w {
			yy := src.Y[y*src.YStride+x]
			i := y*w + x
			// color.YCbCrToRGB is the exported form of the conversion
			// imageutil inlines: the same 91881/22554/46802/116130.
			r, g, bl := color.YCbCrToRGB(yy, cb[i], cr[i])
			o := dst.PixOffset(b.Min.X+x, b.Min.Y+y)
			dst.Pix[o+0], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = r, g, bl, 255
		}
	}
}

// subsampleFactors is how many output samples one chroma sample covers.
func subsampleFactors(r image.YCbCrSubsampleRatio) (hf, vf int) {
	switch r {
	case image.YCbCrSubsampleRatio422:
		return 2, 1
	case image.YCbCrSubsampleRatio420:
		return 2, 2
	case image.YCbCrSubsampleRatio440:
		return 1, 2
	case image.YCbCrSubsampleRatio411:
		return 4, 1
	case image.YCbCrSubsampleRatio410:
		return 4, 2
	}
	return 1, 1
}

// upsample brings one chroma plane up to the full grid.
//
// The 2x1 and 2x2 cases are libjpeg's fancy upsamplers, term for term
// (jdsample.c:405-423 and :455-470): a column sum of near*3+far vertically,
// then near*3+far horizontally with the rounding split asymmetrically between
// the two output columns -- +8 on the even one, +7 on the odd -- and the edge
// columns formed by replicating rather than reading past the plane. Any other
// ratio is repeated, which is what libjpeg does for them too.
func upsample(plane []byte, stride, cw, ch, w, h, hf, vf int) []byte {
	out := make([]byte, w*h)
	if hf != 2 || (vf != 1 && vf != 2) {
		for y := range h {
			sy := min(y/vf, ch-1)
			for x := range w {
				out[y*w+x] = plane[sy*stride+min(x/hf, cw-1)]
			}
		}
		return out
	}
	cols := make([]int, cw)
	for y := range h {
		v := y / vf
		if v >= ch {
			v = ch - 1
		}
		if vf == 2 {
			far := v - 1
			if y&1 == 1 {
				far = v + 1
			}
			far = max(0, min(far, ch-1))
			for c := range cw {
				cols[c] = int(plane[v*stride+c])*3 + int(plane[far*stride+c])
			}
		} else {
			for c := range cw {
				cols[c] = int(plane[v*stride+c]) * 4
			}
		}
		for x := range w {
			u := x / 2
			if u >= cw {
				u = cw - 1
			}
			this := cols[u]
			var val int
			if x&1 == 0 {
				prev := this
				if u > 0 {
					prev = cols[u-1]
				}
				val = (this*3 + prev + 8) >> 4
			} else {
				next := this
				if u+1 < cw {
					next = cols[u+1]
				}
				val = (this*3 + next + 7) >> 4
			}
			out[y*w+x] = clamp8(val)
		}
	}
	return out
}

func clamp8(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}
