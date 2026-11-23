package remux

import (
	"os"
	"testing"

	"hdr10remux/internal/ebml"
	"hdr10remux/internal/mkv"
)

// buildTestInput 生成一个带 HDR10 + 旧字段(ColourSpace/Gamma/Tags)的 MKV。
// confirmed 为 false 时省略 MaxCLL/MaxFALL,使 HDR10 无法被确认。
func buildTestInput(t *testing.T, path string, confirmed bool) {
	t.Helper()
	var b []byte
	b, fin := ebml.AppendMaster(b, mkv.IDEBMLHeader)
	b = ebml.AppendUint(b, mkv.IDEBMLVersion, 1)
	b = ebml.AppendUint(b, mkv.IDEBMLReadVersion, 1)
	b = ebml.AppendUint(b, mkv.IDEBMLMaxIDLength, 4)
	b = ebml.AppendUint(b, mkv.IDEBMLMaxSizeLength, 8)
	b = ebml.AppendString(b, mkv.IDEBMLDocType, "matroska")
	b = ebml.AppendUint(b, mkv.IDEBMLDocTypeVer, 4)
	b = ebml.AppendUint(b, mkv.IDEBMLDocTypeRead, 2)
	b = fin(b)

	b = ebml.AppendID(b, mkv.IDSegment)
	b = ebml.AppendUnknownSize(b)

	b, fin = ebml.AppendMaster(b, mkv.IDInfo)
	b = ebml.AppendUint(b, mkv.IDTimestampScale, 1000000)
	b = ebml.AppendFloat64(b, mkv.IDDuration, 1500.0)
	b = ebml.AppendString(b, mkv.IDMuxingApp, "test-muxer")
	b = ebml.AppendString(b, mkv.IDWritingApp, "test-muxer")
	b = fin(b)

	var tracks []byte
	tracks, tfin := ebml.AppendMaster(tracks, mkv.IDTracks)
	tracks, efin := ebml.AppendMaster(tracks, mkv.IDTrackEntry)
	tracks = ebml.AppendUint(tracks, mkv.IDTrackNumber, 1)
	tracks = ebml.AppendUint(tracks, mkv.IDTrackUID, 111)
	tracks = ebml.AppendUint(tracks, mkv.IDTrackType, mkv.TrackTypeVideo)
	tracks = ebml.AppendString(tracks, mkv.IDCodecID, "V_MPEGH/ISO/HEVC")
	tracks = ebml.AppendBytes(tracks, mkv.IDCodecPrivate, []byte{0x01, 0x02, 0x03})
	tracks, vfin := ebml.AppendMaster(tracks, mkv.IDVideo)
	tracks = ebml.AppendUint(tracks, mkv.IDPixelWidth, 3840)
	tracks = ebml.AppendUint(tracks, mkv.IDPixelHeight, 2160)
	tracks = ebml.AppendBytes(tracks, mkv.IDColourSpaceLegacy, []byte("BT2020"))
	tracks, cfin := ebml.AppendMaster(tracks, mkv.IDColour)
	tracks = ebml.AppendUint(tracks, mkv.IDMatrixCoefficients, 9)
	tracks = ebml.AppendUint(tracks, mkv.IDRange, 1)
	tracks = ebml.AppendUint(tracks, mkv.IDTransferCharac, 16)
	tracks = ebml.AppendUint(tracks, mkv.IDPrimaries, 9)
	tracks = ebml.AppendFloat64(tracks, mkv.IDGammaValue, 2.4)
	if confirmed {
		tracks = ebml.AppendUint(tracks, mkv.IDMaxCLL, 1000)
		tracks = ebml.AppendUint(tracks, mkv.IDMaxFALL, 400)
	}
	tracks, mfin := ebml.AppendMaster(tracks, mkv.IDMasteringMeta)
	tracks = ebml.AppendFloat64(tracks, mkv.IDPrimaryRX, 0.708)
	tracks = ebml.AppendFloat64(tracks, mkv.IDPrimaryRY, 0.292)
	tracks = ebml.AppendFloat64(tracks, mkv.IDPrimaryGX, 0.170)
	tracks = ebml.AppendFloat64(tracks, mkv.IDPrimaryGY, 0.797)
	tracks = ebml.AppendFloat64(tracks, mkv.IDPrimaryBX, 0.131)
	tracks = ebml.AppendFloat64(tracks, mkv.IDPrimaryBY, 0.046)
	tracks = ebml.AppendFloat64(tracks, mkv.IDWhitePointX, 0.3127)
	tracks = ebml.AppendFloat64(tracks, mkv.IDWhitePointY, 0.3290)
	tracks = ebml.AppendFloat64(tracks, mkv.IDLuminanceMax, 1000.0)
	tracks = ebml.AppendFloat64(tracks, mkv.IDLuminanceMin, 0.0001)
	tracks = mfin(tracks)
	tracks = cfin(tracks)
	tracks = vfin(tracks)
	tracks = efin(tracks)

	tracks, efin = ebml.AppendMaster(tracks, mkv.IDTrackEntry)
	tracks = ebml.AppendUint(tracks, mkv.IDTrackNumber, 2)
	tracks = ebml.AppendUint(tracks, mkv.IDTrackUID, 222)
	tracks = ebml.AppendUint(tracks, mkv.IDTrackType, mkv.TrackTypeAudio)
	tracks = ebml.AppendString(tracks, mkv.IDCodecID, "A_AAC")
	tracks = ebml.AppendBytes(tracks, mkv.IDCodecPrivate, []byte{0x11, 0x90})
	tracks, afin := ebml.AppendMaster(tracks, mkv.IDAudio)
	tracks = ebml.AppendFloat64(tracks, mkv.IDSamplingFreq, 48000.0)
	tracks = ebml.AppendUint(tracks, mkv.IDChannels, 2)
	tracks = afin(tracks)
	tracks = efin(tracks)
	tracks = tfin(tracks)
	b = append(b, tracks...)

	b, fin = ebml.AppendMaster(b, mkv.IDTags)
	b = ebml.AppendBytes(b, 0x67C8, []byte("legacy-tag"))
	b = fin(b)

	b, fin = ebml.AppendMaster(b, mkv.IDCluster)
	b = ebml.AppendUint(b, mkv.IDClusterTime, 0)
	b = ebml.AppendBytes(b, mkv.IDSimpleBlock, blockPayload(1, 0, 0x80, []byte("video-frame-0")))
	b = ebml.AppendBytes(b, mkv.IDSimpleBlock, blockPayload(2, 0, 0x02, xiphLace([][]byte{
		[]byte("aud0"), []byte("aud1"), []byte("aud2"),
	})))
	b = fin(b)

	b, fin = ebml.AppendMaster(b, mkv.IDCluster)
	b = ebml.AppendUint(b, mkv.IDClusterTime, 1000)
	b, gfin := ebml.AppendMaster(b, mkv.IDBlockGroup)
	b = ebml.AppendBytes(b, mkv.IDBlock, blockPayload(1, 0, 0x00, []byte("video-frame-1")))
	b = ebml.AppendBytes(b, 0xFB, []byte{0x42})
	b = gfin(b)
	b = ebml.AppendBytes(b, mkv.IDSimpleBlock, blockPayload(2, 10, 0x04, fixedLace([][]byte{
		[]byte("aa"), []byte("bb"),
	})))
	b = ebml.AppendBytes(b, mkv.IDSimpleBlock, blockPayload(2, 30, 0x06, ebmlLace([][]byte{
		[]byte("x"), []byte("yy"), []byte("zzz"),
	})))
	b = fin(b)

	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("写入测试输入失败: %v", err)
	}
}
