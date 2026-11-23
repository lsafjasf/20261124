# WAVEFORMATEXTENSIBLE header inspector

Pure-Go RIFF/WAVE header parser and validator. It parses only header bytes,
classifies WAVEFORMATEXTENSIBLE channel positions, and reports absolute sample
offsets for inconsistencies. It never changes the channel count to fit the
amount of audio present in the file.

## Run

```sh
go run ./cmd/wav-inspect path/to/audio.wav
go run ./cmd/wav-inspect -compact path/to/audio.wav
```

The command prints JSON. Invalid files still produce a header report and exit
with status 1:

```json
{
  "file": "bad-5.1.wav",
  "valid": false,
  "first_sample_offset": 68,
  "issues": [
    {
      "severity": "error",
      "code": "channel_mask_count_mismatch",
      "message": "channel mask 0x0000003F has 6 speaker positions but fmt declares 2 channels",
      "first_sample_offset": 68
    }
  ],
  "header": {
    "layout": "5.1",
    "channels": [
      {"index": 0, "speaker": "front-left", "short_name": "FL", "mask_bit": 1, "bit": 0, "specified": true, "known": true}
    ],
    "data": {
      "offset": 68,
      "first_sample_offset": 68,
      "declared_size": 8,
      "available_size": 8,
      "block_align": 4,
      "frame_count": 2,
      "interleaved_samples": 4
    }
  }
}
```

The example is abbreviated; the real report contains every chunk and channel.

## Rules

- Channel indexes are assigned by increasing channel-mask bit number. The
  report exposes each channel's mask bit, speaker name, and abbreviation.
- Mono accepts a zero mask as "unspecified" or exactly one speaker position.
- Canonical `0x3F`, `0x60F`, `0x3FF`, and `0x63F` masks are named as 5.1,
  5.1-side, 7.1-wide, and 7.1 respectively. Other masks are still mapped
  bit-by-bit and reported as custom layouts.
- A nonzero mask must have exactly `fmt.nChannels` known set bits. Unknown bits
  are retained in the channel table as unknown channels and reported by
  `unknown_channel_mask_bits`.
- The subformat GUID must be one of the known Microsoft KSDATAFORMAT subtypes
  with the correct fixed GUID tail.
- `block_align`, `byte_rate`, sample width, and the data size's frame boundary
  are validated. Frame and interleaved-sample counts come from those header
  fields and `block_align`, not from guessing channels from the file length.
- All offsets are zero-based absolute byte offsets. `data.first_sample_offset`
  is the data payload start. An issue's `first_sample_offset` points at the
  first complete frame that cannot be interpreted safely; header-wide format
  errors point at the data payload start. `-1` means no data payload exists.

## Library

```go
header, err := wav.ParseFile("audio.wav")
```

`Parse(io.ReaderAt, size)` is available for in-memory or custom readers.
Validation failures return `*wav.ValidationError` and the partially filled
`*wav.Header`, so callers can render the same reviewable result.

Run the tests with:

```sh
go test ./...
```
