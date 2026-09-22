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

## Everything else

Identical to the standard library, including `Encode`. Use it exactly as you
would use `image/jpeg`.
