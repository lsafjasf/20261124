package ac3

// ATSC A/52 tables.

// sampleRateTable indexed by fscod (0..2). fscod==3 is reserved.
var sampleRateTable = [3]int{48000, 44100, 32000}

// bitRateTable indexed by frmsizecod>>1, unit kbps.
var bitRateTable = [19]int{
	32, 40, 48, 56, 64, 80, 96, 112, 128, 160,
	192, 224, 256, 320, 384, 448, 512, 576, 640,
}

// frameSizeTable[frmsizecod][fscod] gives the frame size in 16-bit words.
var frameSizeTable = [64][3]int{
	{64, 69, 96}, {64, 70, 96},
	{80, 87, 120}, {80, 88, 120},
	{96, 104, 144}, {96, 105, 144},
	{112, 121, 168}, {112, 122, 168},
	{128, 139, 192}, {128, 140, 192},
	{160, 174, 240}, {160, 175, 240},
	{192, 208, 288}, {192, 209, 288},
	{224, 243, 336}, {224, 244, 336},
	{256, 278, 384}, {256, 279, 384},
	{320, 348, 480}, {320, 349, 480},
	{384, 417, 576}, {384, 418, 576},
	{448, 487, 672}, {448, 488, 672},
	{512, 557, 768}, {512, 558, 768},
	{640, 696, 960}, {640, 697, 960},
	{768, 835, 1152}, {768, 836, 1152},
	{896, 975, 1344}, {896, 976, 1344},
	{1024, 1114, 1536}, {1024, 1115, 1536},
	{1152, 1253, 1728}, {1152, 1254, 1728},
	{1280, 1393, 1920}, {1280, 1394, 1920},
}

// acmod descriptor: number of full-bandwidth channels and a label.
type acmodInfo struct {
	nfchans int
	label   string
}

var acmodTable = [8]acmodInfo{
	{2, "1+1 (dual mono)"},
	{1, "1/0 (mono)"},
	{2, "2/0 (stereo)"},
	{3, "3/0"},
	{3, "2/1"},
	{4, "3/1"},
	{4, "2/2"},
	{5, "3/2"},
}

var bsmodTable = [8]string{
	"main audio service: complete main (CM)",
	"main audio service: music and effects (ME)",
	"associated service: visually impaired (VI)",
	"associated service: hearing impaired (HI)",
	"associated service: dialogue (D)",
	"associated service: commentary (C)",
	"associated service: emergency (E)",
	"associated service: voice over (VO)",
}
