// Copyright 2026, the go-images authors.
// Use of this source code is governed by the BSD-style licence in LICENSE.

package jpeg

import (
	"image"
	"testing"
)

// nearest is what image/internal/imageutil.DrawYCbCr does, and what this fork
// replaces: the chroma sample whose index is the output coordinate divided
// down. It is here as the thing to differ FROM.
func nearest(plane []byte, stride, cw, ch, w, h, hf, vf int) []byte {
	out := make([]byte, w*h)
	for y := range h {
		sy := min(y/vf, ch-1)
		for x := range w {
			out[y*w+x] = plane[sy*stride+min(x/hf, cw-1)]
		}
	}
	return out
}

// TestUpsamplingReadsBetweenTheSamples is the defect this fork exists for.
//
// A chroma plane with one step in it must arrive as a ramp, not a staircase.
// The weights are libjpeg's: the near sample three times the far one, so the
// two output columns either side of a step from 0 to 255 read 64 and 191 and
// not 0 and 255.
func TestUpsamplingReadsBetweenTheSamples(t *testing.T) {
	// Four chroma samples, a step in the middle, one row.
	plane := []byte{0, 0, 255, 255}
	got := upsample(plane, 4, 4, 1, 8, 1, 2, 1)
	want := []byte{0, 0, 0, 64, 191, 255, 255, 255}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d = %d, want %d (whole row %v)", i, got[i], want[i], got)
			break
		}
	}
	near := nearest(plane, 4, 4, 1, 8, 1, 2, 1)
	if string(got) == string(near) {
		t.Fatal("the fancy upsampler gave what repetition gives; it is not running")
	}
	// And repetition is the staircase this replaces.
	if near[3] != 0 || near[4] != 255 {
		t.Errorf("the control is wrong: repetition should step 0 -> 255, got %v", near)
	}
}

// TestUpsamplingReplicatesAtTheEdges: libjpeg forms the first and last output
// columns from the nearest sample alone rather than reading past the plane.
func TestUpsamplingReplicatesAtTheEdges(t *testing.T) {
	plane := []byte{10, 200}
	got := upsample(plane, 2, 2, 1, 4, 1, 2, 1)
	if got[0] != 10 {
		t.Errorf("first column = %d, want the first sample, 10 (row %v)", got[0], got)
	}
	if got[3] != 200 {
		t.Errorf("last column = %d, want the last sample, 200 (row %v)", got[3], got)
	}
}

// TestUpsamplingIsVerticalToo: with 2x2 chroma the near ROW is weighted three
// times the far one before the horizontal pass.
func TestUpsamplingIsVerticalToo(t *testing.T) {
	// Two chroma rows, flat within each, a step between them.
	plane := []byte{0, 0, 255, 255}
	got := upsample(plane, 2, 2, 2, 4, 4, 2, 2)
	row := func(y int) []byte { return got[y*4 : (y+1)*4] }
	if row(0)[1] != 0 {
		t.Errorf("the top row should be the first chroma row: %v", row(0))
	}
	if row(3)[1] != 255 {
		t.Errorf("the bottom row should be the last chroma row: %v", row(3))
	}
	// The two middle rows must lie between, and differ from each other.
	a, b := row(1)[1], row(2)[1]
	if a == 0 || b == 255 || a >= b {
		t.Errorf("the middle rows do not ramp: %d then %d (all %v)", a, b, got)
	}
	if string(got) == string(nearest(plane, 2, 2, 2, 4, 4, 2, 2)) {
		t.Fatal("vertical upsampling gave what repetition gives")
	}
}

// TestUnsubsampledChromaIsUntouched. A 4:4:4 picture has one chroma sample per
// pixel, so there is nothing to read between and the fancy path must be the
// identity -- otherwise this fork would change pictures it has no business
// changing.
func TestUnsubsampledChromaIsUntouched(t *testing.T) {
	plane := []byte{1, 2, 3, 4, 5, 6}
	got := upsample(plane, 3, 3, 2, 3, 2, 1, 1)
	for i := range plane {
		if got[i] != plane[i] {
			t.Fatalf("4:4:4 chroma was altered: %v became %v", plane, got)
		}
	}
}

// TestARatioLibjpegRepeatsIsRepeated. 4:1:1 and 4:1:0 are replicated by
// libjpeg itself, so they are replicated here; claiming to interpolate them
// would be claiming to match something that does not.
func TestARatioLibjpegRepeatsIsRepeated(t *testing.T) {
	plane := []byte{10, 200}
	got := upsample(plane, 2, 2, 1, 8, 1, 4, 1)
	want := nearest(plane, 2, 2, 1, 8, 1, 4, 1)
	if string(got) != string(want) {
		t.Errorf("4:1:1 was interpolated: %v, want %v", got, want)
	}
}

// TestDrawYCbCrFancyFillsEveryPixel is the shape check on the caller: an
// opaque picture out, the right size, nothing left at zero by accident.
func TestDrawYCbCrFancyFillsEveryPixel(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 6, 4), image.YCbCrSubsampleRatio420)
	for i := range src.Y {
		src.Y[i] = 128
	}
	for i := range src.Cb {
		src.Cb[i], src.Cr[i] = 100, 160
	}
	dst := image.NewRGBA(src.Bounds())
	drawYCbCrFancy(dst, src)
	for y := range 4 {
		for x := range 6 {
			o := dst.PixOffset(x, y)
			if dst.Pix[o+3] != 255 {
				t.Fatalf("pixel %d,%d is not opaque", x, y)
			}
			if dst.Pix[o] == 0 && dst.Pix[o+1] == 0 && dst.Pix[o+2] == 0 {
				t.Fatalf("pixel %d,%d was left black", x, y)
			}
		}
	}
}
