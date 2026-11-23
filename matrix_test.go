package aces709

import (
	"math"
	"testing"
)

func TestMatrixInverseRoundTrip(t *testing.T) {
	matrices := []Matrix3{
		BradfordResponse(),
		func() Matrix3 {
			m, err := ACEScgColorSpace().RGBToXYZ()
			if err != nil {
				t.Fatal(err)
			}
			return m
		}(),
		func() Matrix3 {
			m, err := Rec709ColorSpace().XYZToRGB()
			if err != nil {
				t.Fatal(err)
			}
			return m
		}(),
	}
	for _, m := range matrices {
		inverse, err := m.Inverse()
		if err != nil {
			t.Fatalf("Inverse: %v", err)
		}
		product := m.Mul(inverse)
		if delta := maxMatrixDelta(product, Identity3()); delta > 2e-15 {
			t.Fatalf("M*M^-1 identity delta %.18g", delta)
		}
	}
}

func TestMatrixMultiplicationOrder(t *testing.T) {
	translationLike := Matrix3{
		{1, 1, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
	scaleLike := Matrix3{
		{2, 0, 0},
		{0, 3, 0},
		{0, 0, 1},
	}
	got := translationLike.Mul(scaleLike)
	want := Matrix3{
		{2, 3, 0},
		{0, 3, 0},
		{0, 0, 1},
	}
	if !closeMatrix(got, want, 0) {
		t.Fatalf("a.Mul(b) must apply b first:\ngot  %v\nwant %v", got, want)
	}
}

func TestColorSpaceMatricesMapWhitePoints(t *testing.T) {
	acesToXYZ, err := ACEScgColorSpace().RGBToXYZ()
	if err != nil {
		t.Fatal(err)
	}
	d60, err := ACEScgColorSpace().WhitePoint.XYZ()
	if err != nil {
		t.Fatal(err)
	}
	if got := acesToXYZ.ApplyVec(Vec3{1, 1, 1}); !closeVec(got, d60, 2e-15) {
		t.Fatalf("ACEScg white mapped to %v, want %v", got, d60)
	}

	xyzTo709, err := Rec709ColorSpace().XYZToRGB()
	if err != nil {
		t.Fatal(err)
	}
	d65, err := Rec709ColorSpace().WhitePoint.XYZ()
	if err != nil {
		t.Fatal(err)
	}
	if got := xyzTo709.ApplyVec(d65); !closeVec(got, Vec3{1, 1, 1}, 2e-15) {
		t.Fatalf("Rec.709 white mapped to %v, want (1,1,1)", got)
	}
}

func TestBradfordMapsD60ToD65(t *testing.T) {
	m, err := BradfordAdaptation(ACEScgColorSpace().WhitePoint, Rec709ColorSpace().WhitePoint)
	if err != nil {
		t.Fatal(err)
	}
	d60, _ := ACEScgColorSpace().WhitePoint.XYZ()
	d65, _ := Rec709ColorSpace().WhitePoint.XYZ()
	got := m.ApplyVec(d60)
	if !closeVec(got, d65, 3e-15) {
		t.Fatalf("Bradford D60->D65 = %v, want %v", got, d65)
	}
	if math.Abs(got.Y-1) > 1e-15 {
		t.Fatalf("adapted white luminance %g, want 1", got.Y)
	}
}

func TestExpectedMatrixLandmarks(t *testing.T) {
	acesToXYZ, err := ACEScgColorSpace().RGBToXYZ()
	if err != nil {
		t.Fatal(err)
	}
	xyzTo709, err := Rec709ColorSpace().XYZToRGB()
	if err != nil {
		t.Fatal(err)
	}
	adapt, err := BradfordAdaptation(ACEScgColorSpace().WhitePoint, Rec709ColorSpace().WhitePoint)
	if err != nil {
		t.Fatal(err)
	}
	combined := xyzTo709.Mul(adapt).Mul(acesToXYZ)

	// ACEScg neutral must remain neutral after a D60 -> D65 adaptation.
	neutral := combined.ApplyVec(Vec3{0.18, 0.18, 0.18})
	if math.Abs(neutral.X-neutral.Y) > 2e-14 || math.Abs(neutral.Y-neutral.Z) > 2e-14 {
		t.Fatalf("neutral drifted: %v", neutral)
	}
	if math.Abs(neutral.Y-0.18) > 2e-14 {
		t.Fatalf("neutral luminance changed to %g", neutral.Y)
	}
}

func TestPublishedRGBMatrices(t *testing.T) {
	acesToXYZ, err := ACEScgColorSpace().RGBToXYZ()
	if err != nil {
		t.Fatal(err)
	}
	wantACESToXYZ := Matrix3{
		{0.6624541811, 0.1340042065, 0.1561876870},
		{0.2722287168, 0.6740817658, 0.0536895174},
		{-0.0055746495, 0.0040607335, 1.0103391003},
	}
	if delta := maxMatrixDelta(acesToXYZ, wantACESToXYZ); delta > 5e-10 {
		t.Errorf("ACEScg RGB-to-XYZ delta %.3g\n got  %v\n want %v", delta, acesToXYZ, wantACESToXYZ)
	}

	rec709ToXYZ, err := Rec709ColorSpace().RGBToXYZ()
	if err != nil {
		t.Fatal(err)
	}
	wantRec709ToXYZ := Matrix3{
		{0.4123907993, 0.3575843394, 0.1804807884},
		{0.2126390059, 0.7151686788, 0.0721923154},
		{0.0193308187, 0.1191947798, 0.9505321522},
	}
	if delta := maxMatrixDelta(rec709ToXYZ, wantRec709ToXYZ); delta > 5e-10 {
		t.Errorf("Rec.709 RGB-to-XYZ delta %.3g\n got  %v\n want %v", delta, rec709ToXYZ, wantRec709ToXYZ)
	}
}
