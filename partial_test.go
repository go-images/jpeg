// Copyright 2026 The go-images authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found
// in the LICENSE file.

package jpeg

import (
	"bytes"
	"image"
	"os"
	"testing"
)

// sameRows reports the first row on which two images differ, or -1.
func sameRows(a, b image.Image, rows int) int {
	for y := 0; y < rows; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				return y
			}
		}
	}
	return -1
}

// TestThePartialRowsAreTheRealRows.
//
// ⛔ This is the assertion the whole thing stands on: a row COUNTED as arrived
// must hold what the complete decode holds. A count without this is a number that
// tells a caller to draw whatever the buffer happened to contain, which looks like
// a broken image rather than an unfinished one.
//
// The subsampling axis is the one that can make the arithmetic lie: rows per MCU
// row is 8*v0, so 4:2:0 advances sixteen pixel rows where 4:4:4 advances eight. A
// single fixture would have pinned one of those.
func TestThePartialRowsAreTheRealRows(t *testing.T) {
	for _, name := range []string{
		"video-001.jpeg",
		"video-001.q50.410.jpeg",
		"video-001.q50.411.jpeg",
		"video-001.q50.420.jpeg",
		"video-001.q50.422.jpeg",
		"video-001.q50.440.jpeg",
		"video-001.q50.444.jpeg",
		"video-001.221212.jpeg",
		// ⛔ Grayscale takes the other branch entirely: the decoder fills img1
		// (image.Gray) and not img3, and nothing above would have exercised it.
		"video-005.gray.jpeg",
		"video-005.gray.q50.2x2.jpeg",
		// Restart intervals resynchronise inside the scan, which is a different
		// path through the row loop that records the count.
		"video-001.restart2.jpeg",
	} {
		t.Run(name, func(t *testing.T) {
			full, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Skipf("no such fixture: %v", err)
			}
			whole, err := Decode(bytes.NewReader(full))
			if err != nil {
				t.Fatalf("the complete fixture does not decode, so nothing below "+
					"means anything: %v", err)
			}

			seen := 0
			for _, pct := range []int{20, 40, 60, 80, 95} {
				cut := full[:len(full)*pct/100]
				img, rows, err := DecodePartial(bytes.NewReader(cut))
				if err == nil {
					t.Fatalf("%d%% of the file decoded completely, so this cut "+
						"exercises nothing", pct)
				}
				if img == nil {
					continue // Too little to say anything yet; the next cut will say more.
				}
				if rows <= 0 {
					t.Errorf("%d%%: an image came back with %d rows", pct, rows)
					continue
				}
				if img.Bounds() != whole.Bounds() {
					t.Errorf("%d%%: bounds %v, want the full size %v",
						pct, img.Bounds(), whole.Bounds())
				}
				if bad := sameRows(img, whole, rows); bad >= 0 {
					t.Errorf("%d%%: %d rows were offered and row %d already differs "+
						"from the complete decode", pct, rows, bad)
				}
				// ⛔ And the count has to be TIGHT, not merely safe. Under-claiming is
				// invisible to the check above: dropping the chroma factor from the
				// arithmetic halves the count on a 4:2:0 file, every claimed row is
				// still correct, and an ablation doing exactly that PASSED. So: the
				// first row that differs must be within one MCU row of the count.
				// Sixteen pixel rows is the largest an MCU row gets here (8*v0, v0<=2
				// in these fixtures); the slack allows for the row where the data
				// actually stops.
				if firstBad := sameRows(img, whole, whole.Bounds().Dy()); firstBad >= 0 {
					if gap := firstBad - rows; gap > 16 {
						t.Errorf("%d%%: %d rows offered but the picture is right up to row "+
							"%d — %d rows that arrived are being withheld", pct, rows, firstBad, gap)
					}
				}
				if rows < seen {
					t.Errorf("%d%%: %d rows, fewer than the %d a smaller cut offered",
						pct, rows, seen)
				}
				seen = rows
			}
			if seen == 0 {
				t.Errorf("no cut of this file ever produced a row, so the fixture " +
					"proves nothing")
			}
		})
	}
}

// TestACompleteImageSaysEveryRow: the ordinary case still has to work, and it is
// the one a caller cannot distinguish from the partial one except by the count.
func TestACompleteImageSaysEveryRow(t *testing.T) {
	full, err := os.ReadFile("testdata/video-001.jpeg")
	if err != nil {
		t.Fatal(err)
	}
	img, rows, err := DecodePartial(bytes.NewReader(full))
	if err != nil {
		t.Fatalf("DecodePartial on a whole file: %v", err)
	}
	if img == nil {
		t.Fatal("no image")
	}
	if rows != img.Bounds().Dy() {
		t.Errorf("rows = %d, want every one of %d", rows, img.Bounds().Dy())
	}
}

// TestWhatIsRefusedPartially, and the reason for each.
//
// ⛔ A refusal has to be a nil image, not an image with rows=0: a caller that
// checks the image first would draw a frame of whatever the buffer holds. Both are
// asserted, because only one of them is visible in a caller that forgets the other.
func TestWhatIsRefusedPartially(t *testing.T) {
	for _, c := range []struct {
		name string
		file string
		why  string
	}{
		{
			name: "a progressive file",
			file: "video-001.progressive.jpeg",
			why: "its early scans cover the whole frame at low fidelity, so no row " +
				"is final until the last one",
		},
		{
			name: "RGB carried in a JPEG, which needs the same pass",
			file: "video-001.rgb.jpeg",
			why:  "the Adobe marker means the three channels are RGB, not YCbCr",
		},
		{
			name: "CMYK, which needs a pass over the finished image",
			file: "video-001.cmyk.jpeg",
			why:  "converting rows that are partly unwritten invents colour",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			full, err := os.ReadFile("testdata/" + c.file)
			if err != nil {
				t.Skipf("no such fixture: %v", err)
			}
			// The premise: the whole file DOES decode, so a refusal below is about
			// partiality and not about the fixture being unreadable.
			if _, err := Decode(bytes.NewReader(full)); err != nil {
				t.Fatalf("the complete fixture does not decode: %v", err)
			}
			for _, pct := range []int{40, 60, 80} {
				img, rows, err := DecodePartial(bytes.NewReader(full[:len(full)*pct/100]))
				if err == nil {
					t.Fatalf("%d%% decoded completely", pct)
				}
				if img != nil {
					t.Errorf("%d%%: an image was offered although %s", pct, c.why)
				}
				if rows != 0 {
					t.Errorf("%d%%: rows = %d, want 0", pct, rows)
				}
			}
		})
	}
}

// TestNothingIsOfferedBeforeTheFirstRow: a cut inside the header has a size and no
// picture, and offering the allocated buffer would be offering a blank rectangle
// as though it were content.
func TestNothingIsOfferedBeforeTheFirstRow(t *testing.T) {
	full, err := os.ReadFile("testdata/video-001.jpeg")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 2, 20, 100} {
		if n > len(full) {
			break
		}
		img, rows, err := DecodePartial(bytes.NewReader(full[:n]))
		if err == nil {
			t.Errorf("%d bytes decoded completely", n)
		}
		if img != nil || rows != 0 {
			t.Errorf("%d bytes offered an image with %d rows", n, rows)
		}
	}
}
