package mkv

// Matroska / EBML 元素 ID 常量(仅列出本工具用到的部分)。
const (
	IDEBMLHeader        uint32 = 0x1A45DFA3
	IDEBMLVersion       uint32 = 0x4286
	IDEBMLReadVersion   uint32 = 0x42F7
	IDEBMLMaxIDLength   uint32 = 0x42F2
	IDEBMLMaxSizeLength uint32 = 0x42F3
	IDEBMLDocType       uint32 = 0x4282
	IDEBMLDocTypeVer    uint32 = 0x4287
	IDEBMLDocTypeRead   uint32 = 0x4285

	IDSegment        uint32 = 0x18538067
	IDSeekHead       uint32 = 0x114D9B74
	IDInfo           uint32 = 0x1549A966
	IDTimestampScale uint32 = 0x2AD7B1
	IDDuration       uint32 = 0x4489
	IDMuxingApp      uint32 = 0x4D80
	IDWritingApp     uint32 = 0x5741
	IDTracks         uint32 = 0x1654AE6B
	IDTrackEntry     uint32 = 0xAE
	IDTrackNumber    uint32 = 0xD7
	IDTrackUID       uint32 = 0x73C5
	IDTrackType      uint32 = 0x83
	IDFlagEnabled    uint32 = 0xB9
	IDFlagDefault    uint32 = 0x88
	IDFlagLacing     uint32 = 0x9C
	IDCodecID        uint32 = 0x86
	IDCodecPrivate   uint32 = 0x63A2
	IDTrackName      uint32 = 0x536E
	IDTrackLanguage  uint32 = 0x22B59C
	IDVideo          uint32 = 0xE0
	IDPixelWidth     uint32 = 0xB0
	IDPixelHeight    uint32 = 0xBA
	IDAudio          uint32 = 0xE1
	IDSamplingFreq   uint32 = 0xB5
	IDChannels       uint32 = 0x9F
	IDBitDepth       uint32 = 0x6264
	IDCluster        uint32 = 0x1F43B675
	IDClusterTime    uint32 = 0xE7
	IDSimpleBlock    uint32 = 0xA3
	IDBlockGroup     uint32 = 0xA0
	IDBlock          uint32 = 0xA1
	IDBlockDuration  uint32 = 0x9B

	// Colour 及其子元素(HDR10 目标字段)。
	IDColour             uint32 = 0x55B0
	IDMatrixCoefficients uint32 = 0x55B1
	IDRange              uint32 = 0x55B9
	IDTransferCharac     uint32 = 0x55BA
	IDPrimaries          uint32 = 0x55BB
	IDMaxCLL             uint32 = 0x55BC
	IDMaxFALL            uint32 = 0x55BD
	IDMasteringMeta      uint32 = 0x55D0
	IDPrimaryRX          uint32 = 0x55D1
	IDPrimaryRY          uint32 = 0x55D2
	IDPrimaryGX          uint32 = 0x55D3
	IDPrimaryGY          uint32 = 0x55D4
	IDPrimaryBX          uint32 = 0x55D5
	IDPrimaryBY          uint32 = 0x55D6
	IDWhitePointX        uint32 = 0x55D7
	IDWhitePointY        uint32 = 0x55D8
	IDLuminanceMax       uint32 = 0x55D9
	IDLuminanceMin       uint32 = 0x55DA

	// 旧字段:重封装时必须彻底消失。
	IDColourSpaceLegacy uint32 = 0x2EB524 // 已废弃的 ColourSpace
	IDGammaValue        uint32 = 0x2FB523 // 已废弃的 Gamma
	IDTags              uint32 = 0x1254C367
	IDChapters          uint32 = 0x1043A770
	IDAttachments       uint32 = 0x1941A469
	IDCues              uint32 = 0x1C53BB6B
)

// 轨道类型。
const (
	TrackTypeVideo uint64 = 1
	TrackTypeAudio uint64 = 2
)

// LegacyIDs 返回重封装后不允许再出现的元素 ID 列表。
func LegacyIDs() []uint32 {
	return []uint32{
		IDColourSpaceLegacy,
		IDGammaValue,
		IDTags,
		IDChapters,
		IDAttachments,
	}
}
