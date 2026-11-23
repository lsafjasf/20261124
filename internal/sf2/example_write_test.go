package sf2

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteExamples renders the in-memory fixtures to disk so the CLI can be
// exercised against real RIFF/sfbk containers. It only runs when SF2_OUT is
// set, keeping normal test runs side-effect free.
func TestWriteExamples(t *testing.T) {
	dir := os.Getenv("SF2_OUT")
	if dir == "" {
		t.Skip("SF2_OUT not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(name string, b *sf2Builder) {
		if err := os.WriteFile(filepath.Join(dir, name), b.bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("valid.sf2", validFile())

	missing := validFile()
	missing.igens[2] = genIndex(GenSampleIDClassic, 42)
	write("missing-ref.sf2", missing)

	loop := validFile()
	loop.samples[1].LoopStart = 30
	loop.samples[1].LoopEnd = 50
	write("overlap-loop.sf2", loop)

	root := validFile()
	root.samples[0].RootKey = 128
	write("bad-root.sf2", root)
}
