// Command fixture generates the deterministic SDR/HDR review pair.
package main

import (
	"os"

	"vxcheck/internal/fixture"
)

func main() {
	spec := fixture.Default()
	must(os.MkdirAll("artifacts", 0o755))

	sdr, err := os.Create("artifacts/sdr.vx")
	must(err)
	must(fixture.WriteSDR(sdr, spec))
	must(sdr.Close())

	hdr, err := os.Create("artifacts/hdr.vx")
	must(err)
	must(fixture.WriteHDR(hdr, spec))
	must(hdr.Close())
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
