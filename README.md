# jpeg

> **A fork of Go's `image/jpeg`**, under the same BSD-3-Clause licence.
> The import path is `github.com/go-images/jpeg`. Every change is stated in
> [NOTICE](NOTICE), and Go's own test suite passes here unmodified in
> substance.

**What differs:** a **four-component** JPEG's chroma is upsampled the way
libjpeg does — reading between the samples — rather than by repeating each
one.

`image/jpeg` merges the four planes itself, through
`image/internal/imageutil.DrawYCbCr`, which takes the chroma sample whose
index is the output coordinate divided down. A caller receives an
`*image.CMYK` whose chroma is a staircase and **cannot do better, because the
planes are gone**. A three-component picture is unaffected: it still comes
back as an `*image.YCbCr` with its planes intact.

Measured against poppler, which is libjpeg, on a 258×258 YCCK picture with
2×2 chroma — 266 256 samples:

| | C | M | Y | **K** | within one level |
|---|---:|---:|---:|---:|---:|
| `image/jpeg` | 36 | 18 | 21 | **1** | 91.16% |
| this | **2** | **2** | **3** | **1** | **99.90%** |

The black plate is the control: it is carried at full resolution, is never
upsampled, and is 1 both before and after. A decoder that simply differed
would have moved it too.

## Why a fork

The alternative is to reconstruct the chroma from what `image/jpeg` hands
back. That was measured too: the per-block chroma can be recovered exactly
where nothing clipped — 14 215 of 14 244 blocks at zero spread — but 7.8% of
blocks have every pixel clipped on some channel and their chroma is gone, so
the reconstruction stops at **26** levels where this reaches **3**.

## `DecodePartial`: what arrived, and how much of it is real

```go
img, rows, err := jpeg.DecodePartial(r)
```

The image at its full declared size, the number of pixel rows that are
complete counting from the top, and the error that stopped the decode — `nil`
when everything arrived. Rows past the count hold whatever the image was
allocated with, so a caller draws the first `rows` and nothing else.

`Decode` is all-or-nothing, and for a file still arriving that is the same as
having nothing. Measured on a 750 KB photograph truncated at three fractions,
the standard decoder returns a nil image and `short Huffman data` every time —
with three quarters of the picture on disk. `image/png` and `image/gif` answer
the same way, so this is a gap in the shape of every decoder rather than in one
of them.

What `DecodePartial` returns for the same file, 3840×2560:

| bytes given | rows offered | |
|---:|---:|---:|
| 10% | 256 | 10.0% |
| 25% | 616 | 24.1% |
| 50% | 1272 | 49.7% |
| 75% | 1936 | 75.6% |
| 100% | 2560 | 100% |

**The count names finished rows and never the row being decoded.** A truncated
stream stops inside an MCU row, and a caller handed that row would draw a band
of half-decoded blocks — visibly wrong in a way that reads as a broken image
rather than an unfinished one.

Three shapes are deliberately not served partially. Each returns a nil image
along with the error, so a caller that checks either one is safe:

- **A progressive JPEG.** Its early scans cover the whole frame at low
  fidelity, so no row is final until the last scan and "rows from the top"
  cannot describe it. Measured over 300 files from one real library, 5.7% were
  progressive and 94.3% sequential.
- **CMYK, and RGB carried in a JPEG** (the Adobe APP14 marker). Both need a
  conversion pass over the finished image, and running it across rows that are
  partly unwritten produces colour nobody sent.
- **Anything that failed before the first MCU row completed.** There is a size
  but no picture.

## Everything else

Identical to the standard library, including `Encode`. Use it exactly as you
would use `image/jpeg`.
