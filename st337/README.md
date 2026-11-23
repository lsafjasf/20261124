# st337 — SMPTE ST 337 / IEC 61937 burst parser

Parses a raw AES3 word stream (16-bit words, one word per subframe sample
slot) and splits it into PCM, data-burst and error segments. Standard
library only.

## Usage

```
go build -o st337 .
./st337 [flags] <raw-file>        # or stdin with "-"
```

Flags:

- `-be` — input words are big-endian (default little-endian)
- `-channels N` — subframes per AES3 frame for frame numbers (default 2)
- `-dump DIR` — write raw payloads of unknown data types to DIR
- `-dump-all` — also dump payloads of known data types

## What it detects

- Burst preamble Pa=0xF872 / Pb=0x4E1F, duplicated per channel in
  interleaved stereo or single in mono/deinterleaved captures.
- Pc decoding: data type (bits 0-4), type-dependent info (5-6), error
  flag (7), stream number (8-12), burst mode (13-15).
- Pd payload length in bits; payload rounded up to whole 16-bit words,
  trailing zero pad bits reported.
- Zero stuffing words after the payload.
- Truncation: payload cut by end of stream or by a new preamble.
- Illegal sync word: Pa not followed by Pb.
- Byte-order error: byte-swapped preamble 0x72F8 / 0x1F4E.
- Unknown data types keep their raw payload (see `-dump`).
- Ordinary PCM is only reported as PCM; a burst requires the full
  Pa/Pb sync pair, so audio is never misreported as a data burst.

## Output

One line per segment with word and frame sample intervals:

```
[1] BURST words [128, 168)  frames [64, 84)  type=0x01 (AC-3) mode=0 stream=0 len=256 bits (16 words, 0 pad bits) stuffing=16 words
```

Run tests with `go test ./...`.
