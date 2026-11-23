package chromasiting

// crossing finds where a row of samples falls through mid, scanning for the
// first high-to-low transition and interpolating linearly between the two
// straddling samples. The result is in sample-index units of the row.
func crossing(row []float64, mid float64) (float64, bool) {
	for i := 0; i+1 < len(row); i++ {
		a, b := row[i], row[i+1]
		if a >= mid && b < mid && a != b {
			return float64(i) + (a-mid)/(a-b), true
		}
	}
	return 0, false
}

func rowRange(row []float64) (lo, hi float64) {
	lo, hi = row[0], row[0]
	for _, v := range row[1:] {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// BoundaryBeforeUpsampling measures the red/blue edge position implied by
// the subsampled Cr plane, mapped back to luma coordinates via the siting:
// chroma index cx corresponds to luma x = 2*cx + ox.
func BoundaryBeforeUpsampling(img Image420, s Siting, row int) (float64, bool) {
	ox, _ := s.Offset()
	r := img.Cr.Row(row)
	lo, hi := rowRange(r)
	cx, ok := crossing(r, (lo+hi)/2)
	if !ok {
		return 0, false
	}
	return 2*cx + ox, true
}

// BoundaryAfterUpsampling measures the red/blue edge position in the
// reconstructed full-resolution Cr plane, in luma pixel coordinates.
func BoundaryAfterUpsampling(img Image444, row int) (float64, bool) {
	r := img.Cr.Row(row)
	lo, hi := rowRange(r)
	return crossing(r, (lo+hi)/2)
}

// BoundaryReport compares the edge position before and after upsampling for
// one sample and one siting assumption.
type BoundaryReport struct {
	Sample    string
	Siting    Siting
	True      float64
	Before    float64
	After     float64
	DeltaTrue float64 // After - True
}

// MeasureBoundary runs the full pipeline for a sample: subsample with the
// given siting, measure the edge in the 4:2:0 domain, upsample with the same
// siting, and measure again in the 4:4:4 domain.
func MeasureBoundary(smp Sample, s Siting) BoundaryReport {
	sub := Subsample420(smp.Img, s)
	up := Upsample444(sub, s)
	row := smp.Img.Y.H / 2
	before, _ := BoundaryBeforeUpsampling(sub, s, row/2)
	after, _ := BoundaryAfterUpsampling(up, row)
	return BoundaryReport{
		Sample:    smp.Name,
		Siting:    s,
		True:      smp.BoundaryX,
		Before:    before,
		After:     after,
		DeltaTrue: after - smp.BoundaryX,
	}
}

// MeasureMismatch subsamples with the true siting but upsamples assuming a
// different one — the real-world consequence of trusting the wrong
// container field. It returns the reconstructed edge position.
func MeasureMismatch(smp Sample, trueSiting, assumed Siting) float64 {
	sub := Subsample420(smp.Img, trueSiting)
	up := Upsample444(sub, assumed)
	after, _ := BoundaryAfterUpsampling(up, smp.Img.Y.H/2)
	return after
}
