package ac3

// SampleRateCode maps fscod to the sample rate in Hz.
// fscod == 3 is reserved in AC-3 and must be rejected.
var SampleRateCode = [3]int{48000, 44100, 32000}

// frameSizeWords[frmsizecod][fscod] holds the frame size in 16-bit words
// per ATSC A/52 Table 5.18. A frame is 2*words bytes long.
var frameSizeWords = [38][3]int{
	{64, 69, 96},
	{64, 70, 96},
	{80, 87, 120},
	{80, 88, 120},
	{96, 104, 144},
	{96, 105, 144},
	{112, 121, 168},
	{112, 122, 168},
	{128, 139, 192},
	{128, 140, 192},
	{160, 174, 240},
	{160, 175, 240},
	{192, 208, 288},
	{192, 209, 288},
	{224, 243, 336},
	{224, 244, 336},
	{256, 278, 384},
	{256, 279, 384},
	{320, 348, 480},
	{320, 349, 480},
	{384, 417, 576},
	{384, 418, 576},
	{448, 487, 672},
	{448, 488, 672},
	{512, 557, 768},
	{512, 558, 768},
	{640, 696, 960},
	{640, 697, 960},
	{768, 835, 1152},
	{768, 836, 1152},
	{896, 975, 1344},
	{896, 976, 1344},
	{1024, 1114, 1536},
	{1024, 1115, 1536},
	{1152, 1253, 1728},
	{1152, 1254, 1728},
	{1280, 1393, 1920},
	{1280, 1394, 1920},
}

// frameSizeBytes returns the frame length in bytes for a given frame-size
// code and sample-rate code, or -1 when the code is out of range.
func frameSizeBytes(frmsizecod, fscod int) int {
	if frmsizecod < 0 || frmsizecod >= len(frameSizeWords) || fscod < 0 || fscod > 2 {
		return -1
	}
	return 2 * frameSizeWords[frmsizecod][fscod]
}

// crcSplitByte returns q, the byte offset separating the CRC1 and CRC2
// regions. ATSC A/52 defines the 5/8 point on the even frmsizecod pair's word
// count, so odd-word frames (44.1 kHz padding) share the split of their even
// sibling: q = 2 * floor(5 * W_even / 8).
func crcSplitByte(frameSize int) int {
	evenWords := (frameSize / 2) &^ 1
	return 2 * (5 * evenWords / 8)
}
