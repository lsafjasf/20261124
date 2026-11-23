package tagfix

import (
	"testing"

	"tagfix/internal/sample"
)

type spsOptions = sample.SPSOptions

type fixtureOptions struct {
	sps       sample.SPSOptions
	noSPS     bool
	noColr    bool
	container ColorTags
	mdat      []byte
}

func makeMP4(t *testing.T, options fixtureOptions) []byte {
	t.Helper()
	return sample.MP4(sample.Options{
		SPS:    options.sps,
		NoSPS:  options.noSPS,
		NoColr: options.noColr,
		Container: sample.Color{
			Primaries: options.container.Primaries,
			Transfer:  options.container.Transfer,
			Matrix:    options.container.Matrix,
			FullRange: options.container.FullRange,
		},
		MDAT: options.mdat,
	})
}
