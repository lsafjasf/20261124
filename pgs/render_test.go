package pgs

import (
	"bytes"
	"image"
	"strings"
	"testing"
)

const (
	whiteAlpha50 = 1
	grayAlpha50  = 2
	grayAlpha25  = 3
)

func testScenario(t *testing.T) *Stream {
	t.Helper()
	var b supBuilder

	const pts0 uint32 = 0
	objects := []compositionObject{
		{id: 1, windowID: 0, x: 580, y: 170},
		{id: 2, windowID: 0, x: 620, y: 170},
		{id: 3, windowID: 1, x: 1850, y: 920},
	}
	b.segment(pts0, SegmentPresentation,
		pcsPayload(1920, 1080, 1, 2, false, 0, objects))
	b.segment(pts0, SegmentWindow, wdsPayload(
		parsedWindow{id: 0, rect: Rect{X: 600, Y: 150, Width: 300, Height: 300}},
		parsedWindow{id: 1, rect: Rect{X: 1800, Y: 900, Width: 300, Height: 300}},
	))
	b.segment(pts0, SegmentPalette, pdsPayload(0, 0,
		testPaletteEntry{index: whiteAlpha50, y: 235, cr: 128, cb: 128, t: 128},
		testPaletteEntry{index: grayAlpha50, y: 128, cr: 128, cb: 128, t: 128},
		testPaletteEntry{index: grayAlpha25, y: 180, cr: 128, cb: 128, t: 64},
	))
	b.segment(pts0, SegmentObject, odsPayload(1, 0, 0xc0, 50, 20,
		solidPixels(50, 20, whiteAlpha50)))
	b.segment(pts0, SegmentObject, odsPayload(2, 0, 0xc0, 20, 20,
		solidPixels(20, 20, grayAlpha50)))
	b.segment(pts0, SegmentObject, odsPayload(3, 0, 0xc0, 100, 100,
		solidPixels(100, 100, grayAlpha25)))
	b.end(pts0)

	const pts1 uint32 = 90000
	b.segment(pts1, SegmentPresentation,
		pcsPayload(1920, 1080, 2, 0, true, 0, nil))
	b.segment(pts1, SegmentPalette, pdsPayload(0, 1,
		testPaletteEntry{index: grayAlpha50, y: 128, cr: 128, cb: 128, t: 255}))
	b.end(pts1)

	const pts2 uint32 = 180000
	b.segment(pts2, SegmentPresentation,
		pcsPayload(1920, 1080, 3, 0, true, 0, nil))
	b.segment(pts2, SegmentPalette, pdsPayload(0, 2,
		testPaletteEntry{index: grayAlpha50, y: 128, cr: 128, cb: 128, t: 0}))
	b.end(pts2)

	stream, err := ReadSUP(bytes.NewReader(b.bytes()))
	if err != nil {
		t.Fatalf("ReadSUP: %v", err)
	}
	return stream
}

func pixelAt(img *image.RGBA, x, y int) (uint8, uint8, uint8, uint8) {
	off := y*img.Stride + x*4
	return img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3]
}

func TestPaletteAlphaAndTwoPaletteUpdates(t *testing.T) {
	stream := testScenario(t)

	first, err := stream.RenderAt(500 * 1000 * 1000)
	if err != nil {
		t.Fatalf("render at 0.5s: %v", err)
	}
	r, g, b, a := pixelAt(first.Image, 635, 180)
	if a != 128 || r < 60 || r > 70 || g != r || b != r {
		t.Fatalf("initial palette alpha pixel = (%d,%d,%d,%d), want gray with alpha 128", r, g, b, a)
	}

	updated, err := stream.RenderAt(1500 * 1000 * 1000)
	if err != nil {
		t.Fatalf("render after first palette update: %v", err)
	}
	r, g, b, a = pixelAt(updated.Image, 635, 180)
	if a != 255 || r != 130 || g != 130 || b != 130 {
		t.Fatalf("first palette update pixel = (%d,%d,%d,%d), want (130,130,130,255)", r, g, b, a)
	}
	if len(updated.Objects) != 3 {
		t.Fatalf("palette update lost composition objects: got %d", len(updated.Objects))
	}

	cleared, err := stream.RenderAt(2500 * 1000 * 1000)
	if err != nil {
		t.Fatalf("render after second palette update: %v", err)
	}
	_, _, _, a = pixelAt(cleared.Image, 635, 180)
	if a != 0 {
		t.Fatalf("second palette update alpha = %d, want 0", a)
	}
	_, _, _, a = pixelAt(cleared.Image, 605, 180)
	if a != 128 {
		t.Fatalf("unrelated palette entry changed: alpha = %d, want 128", a)
	}
	_, _, _, a = pixelAt(cleared.Image, 625, 180)
	if a != 128 {
		t.Fatalf("transparent overlay should reveal object 1: alpha = %d, want 128", a)
	}

	beforeUpdate, err := stream.RenderAtPTS(89999)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := pixelAt(beforeUpdate.Image, 635, 180); a != 128 {
		t.Fatalf("PTS 89999 selected update early: alpha = %d", a)
	}
	exactUpdate, err := stream.RenderAtPTS(90000)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := pixelAt(exactUpdate.Image, 635, 180); a != 255 {
		t.Fatalf("exact PTS 90000 did not select update: alpha = %d", a)
	}
}

func TestWindowObjectClippingAndOverlayOrder(t *testing.T) {
	stream := testScenario(t)
	frame, err := stream.Render(RenderOptions{At: 500 * 1000 * 1000})
	if err != nil {
		t.Fatal(err)
	}

	if got := frame.Objects[0].Clipped; got != (Rect{X: 600, Y: 170, Width: 30, Height: 20}) {
		t.Fatalf("object 1 clip = %+v", got)
	}
	if got := frame.Objects[1].Clipped; got != (Rect{X: 620, Y: 170, Width: 20, Height: 20}) {
		t.Fatalf("object 2 clip = %+v", got)
	}
	if got := frame.Objects[2].Clipped; got != (Rect{X: 1850, Y: 920, Width: 70, Height: 100}) {
		t.Fatalf("object 3 clip = %+v", got)
	}
	if _, _, _, a := pixelAt(frame.Image, 595, 180); a != 0 {
		t.Fatalf("pixel left of window has alpha %d, want 0", a)
	}
	if _, _, _, a := pixelAt(frame.Image, 605, 180); a != 128 {
		t.Fatalf("window-clipped object alpha = %d, want 128", a)
	}

	r, g, b, a := pixelAt(frame.Image, 635, 180)
	if a != 128 || r < 60 || r > 70 || g != r || b != r {
		t.Fatalf("overlay pixel = (%d,%d,%d,%d), want semi-transparent gray", r, g, b, a)
	}
}

func TestScalingMetadataAndOutputSize(t *testing.T) {
	stream := testScenario(t)
	frame, err := stream.Render(RenderOptions{At: 500 * 1000 * 1000, Width: 1280})
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width != 1280 || frame.Height != 720 {
		t.Fatalf("output size = %dx%d, want 1280x720", frame.Width, frame.Height)
	}
	if frame.Image.Bounds().Dx() != 1280 || frame.Image.Bounds().Dy() != 720 {
		t.Fatalf("image size = %v", frame.Image.Bounds())
	}

	w0, w1 := frame.Windows[0], frame.Windows[1]
	if w0.Output != (Rect{X: 400, Y: 100, Width: 200, Height: 200}) {
		t.Fatalf("window 0 output = %+v", w0.Output)
	}
	if w1.Output != (Rect{X: 1200, Y: 600, Width: 200, Height: 200}) {
		t.Fatalf("window 1 output = %+v", w1.Output)
	}
	if w1.Clipped != (Rect{X: 1800, Y: 900, Width: 120, Height: 180}) {
		t.Fatalf("window 1 canvas clip = %+v", w1.Clipped)
	}
	if w1.OutputClipped != (Rect{X: 1200, Y: 600, Width: 80, Height: 120}) {
		t.Fatalf("window 1 output clip = %+v", w1.OutputClipped)
	}
	if got := frame.Objects[2].Output; got != (Rect{X: 1233, Y: 613, Width: 47, Height: 67}) {
		t.Fatalf("out-of-bounds object output = %+v", got)
	}
	if _, _, _, a := pixelAt(frame.Image, 1240, 620); a == 0 {
		t.Fatal("scaled out-of-bounds object did not reach the output")
	}
}

func TestReadSUPRejectsBadMagic(t *testing.T) {
	_, err := ReadSUP(strings.NewReader("not a PGS stream"))
	if err == nil {
		t.Fatal("expected bad magic error")
	}
}
