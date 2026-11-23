package tagfix

import "fmt"

// ColorTags is the normalized set of color metadata that must exist in both
// the MP4 container and every active H.264 SPS.
type ColorTags struct {
	Primaries uint8 `json:"colour_primaries"`
	Transfer  uint8 `json:"transfer_characteristics"`
	Matrix    uint8 `json:"matrix_coefficients"`
	FullRange bool  `json:"video_full_range"`
}

var validPrimaries = map[uint8]bool{
	1: true, 2: true, 4: true, 5: true, 6: true, 7: true, 8: true,
	9: true, 10: true, 11: true, 12: true, 22: true,
}

var validTransfer = map[uint8]bool{
	1: true, 2: true, 4: true, 5: true, 6: true, 7: true, 8: true,
	9: true, 10: true, 11: true, 12: true, 13: true, 14: true,
	15: true, 16: true, 17: true, 18: true,
}

var validMatrix = map[uint8]bool{
	0: true, 1: true, 2: true, 4: true, 5: true, 6: true, 7: true,
	8: true, 9: true, 10: true, 11: true, 12: true, 13: true, 14: true,
}

// Validate rejects reserved code points and HDR transfer functions paired with
// SDR-only primaries. These combinations cannot be made meaningful merely by
// copying the same numbers into both locations.
func (t ColorTags) Validate() error {
	if !validPrimaries[t.Primaries] {
		return fmt.Errorf("%w: reserved or unknown colour_primaries %d",
			ErrInvalidTags, t.Primaries)
	}
	if !validTransfer[t.Transfer] {
		return fmt.Errorf("%w: reserved or unknown transfer_characteristics %d",
			ErrInvalidTags, t.Transfer)
	}
	if !validMatrix[t.Matrix] {
		return fmt.Errorf("%w: reserved or unknown matrix_coefficients %d",
			ErrInvalidTags, t.Matrix)
	}
	if (t.Transfer == 16 || t.Transfer == 18) &&
		t.Primaries != 2 && t.Primaries != 9 {
		return fmt.Errorf("%w: PQ/HLG transfer %d requires BT.2020 or unspecified primaries, got %d",
			ErrInvalidTags, t.Transfer, t.Primaries)
	}
	if t.Matrix == 0 && !t.FullRange {
		return fmt.Errorf("%w: identity matrix requires full-range signalling",
			ErrInvalidTags)
	}
	return nil
}

func (t ColorTags) equal(other ColorTags) bool {
	return t == other
}

// String is intentionally compact for report messages.
func (t ColorTags) String() string {
	rangeName := "limited"
	if t.FullRange {
		rangeName = "full"
	}
	return fmt.Sprintf("primaries=%d transfer=%d matrix=%d range=%s",
		t.Primaries, t.Transfer, t.Matrix, rangeName)
}
