# 20261124

## ac3meta

AC-3 (ATSC A/52) pre-ingest metadata inspector. Pure Go standard library,
no PCM decoding.

- Reads per-frame dialnorm, bsmod, DRC (compr / block-0 dynrng) and
  sampling parameters (sample rate, bit rate, frame size, channel config).
- Computes gain targets: normalization to a loudness target and
  compensation gain for a hypothetical dialnorm rewrite.
- Locates CRC errors, stream resyncs (concatenated frames / garbage) and
  encoding parameter changes by frame number.

```
go run ./cmd/ac3meta [-target LKFS] [-set-dialnorm N] [-v] file.ac3
go test ./...
```
