package chromasiting

import "testing"

func TestSitingOffsets(t *testing.T) {
	cases := []struct {
		s      Siting
		ox, oy float64
	}{
		{SitingMPEG2, 0.0, 0.5},
		{SitingJPEG, 0.5, 0.5},
		{SitingCenter, 0.5, 0.5},
	}
	for _, c := range cases {
		ox, oy := c.s.Offset()
		if ox != c.ox || oy != c.oy {
			t.Errorf("%v offset = (%v,%v), want (%v,%v)", c.s, ox, oy, c.ox, c.oy)
		}
	}
}

func TestSamePosition(t *testing.T) {
	if !SitingJPEG.SamePosition(SitingCenter) {
		t.Error("JPEG and Center must be the same physical position")
	}
	if SitingMPEG2.SamePosition(SitingJPEG) {
		t.Error("MPEG-2 and JPEG must differ")
	}
	if SitingUnknown.SamePosition(SitingMPEG2) {
		t.Error("unknown never matches")
	}
}

func intp(v int) *int       { return &v }
func u64p(v uint64) *uint64 { return &v }

func TestIdentifyAgreement(t *testing.T) {
	res := Identify(ContainerFields{
		H264ChromaSampleLocationType: intp(0),
		MKVChromaSitingHorz:          u64p(1),
		MKVChromaSitingVert:          u64p(2),
		CodecID:                      "mpeg2video",
	})
	if res.Status != StatusResolved || res.Siting != SitingMPEG2 {
		t.Fatalf("got %v %v, want resolved MPEG-2", res.Status, res.Siting)
	}
	if len(res.Claims) != 3 {
		t.Fatalf("want 3 claims, got %d", len(res.Claims))
	}
}

func TestIdentifyEquivalentLabelsNotConflict(t *testing.T) {
	// H.264 "center" and MJPEG codec default are the same physical position.
	res := Identify(ContainerFields{
		H264ChromaSampleLocationType: intp(1),
		CodecID:                      "mjpeg",
	})
	if res.Status != StatusResolved {
		t.Fatalf("JPEG/Center equivalence should resolve, got %v", res.Status)
	}
}

func TestIdentifyConflictGoesToManualReview(t *testing.T) {
	res := Identify(ContainerFields{
		H264ChromaSampleLocationType: intp(0),
		CodecID:                      "mjpeg",
	})
	if res.Status != StatusManualReview {
		t.Fatalf("conflict must be manual review, got %v", res.Status)
	}
	if len(res.Claims) != 2 {
		t.Fatalf("all conflicting claims must be listed, got %d", len(res.Claims))
	}
}

func TestIdentifyNoMajorityVote(t *testing.T) {
	// Three fields say MPEG-2, one says JPEG: still manual review.
	res := Identify(ContainerFields{
		H264ChromaSampleLocationType: intp(0),
		MKVChromaSitingHorz:          u64p(1),
		MKVChromaSitingVert:          u64p(2),
		AV1ChromaSamplePosition:      intp(1),
		CodecID:                      "mjpeg",
	})
	if res.Status != StatusManualReview {
		t.Fatalf("majority must not auto-win, got %v (siting %v)", res.Status, res.Siting)
	}
	if len(res.Claims) != 4 {
		t.Fatalf("want 4 claims listed, got %d", len(res.Claims))
	}
}

func TestIdentifyUnsupportedValueForcesManualReview(t *testing.T) {
	res := Identify(ContainerFields{H264ChromaSampleLocationType: intp(2)})
	if res.Status != StatusManualReview {
		t.Fatalf("unsupported VUI value must be manual review, got %v", res.Status)
	}
}

func TestIdentifyUnspecified(t *testing.T) {
	res := Identify(ContainerFields{CodecID: "h264"})
	if res.Status != StatusUnspecified {
		t.Fatalf("no fields -> unspecified, got %v", res.Status)
	}
}
