package au

import "testing"

func FuzzParse(f *testing.F) {
	f.Add([]byte{})
	f.Add(makeAU(24, 0, uint32(EncodingMuLaw), 8000, 1, nil))
	f.Add(makeAU(32, 6, uint32(EncodingLinear16), 44100, 2,
		[]byte{0, 1, 0, 2, 0, 3}))
	f.Add(makeAU(24, UnknownDataSize, 99, 8000, 1,
		[]byte{0xde, 0xad, 0xbe, 0xef}))

	f.Fuzz(func(t *testing.T, data []byte) {
		file, err := Parse(data)
		if err != nil {
			return
		}
		if file.Format.Kind != KindUnknown &&
			file.Frames*file.Format.BlockAlign != uint64(len(file.Data)) {
			t.Fatalf("inconsistent payload length: frames=%d block=%d data=%d",
				file.Frames, file.Format.BlockAlign, len(file.Data))
		}
		_, _ = file.Samples()
	})
}
