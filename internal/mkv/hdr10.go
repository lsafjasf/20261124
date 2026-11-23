package mkv

import "fmt"

// HDR10 相关的枚举取值(C/MatrixCoefficients 等遵循 Matroska 规范)。
const (
	PrimariesBT2020     uint64 = 9  // BT.2020
	TransferPQ          uint64 = 16 // SMPTE ST 2084 (PQ)
	MatrixBT2020NCL     uint64 = 9  // BT.2020 非恒定亮度
	MatrixBT2020CL      uint64 = 10 // BT.2020 恒定亮度
	masteringFieldCount        = 10
)

// MasteringMetadata 对应 SMPTE ST 2086  mastering display colour volume。
type MasteringMetadata struct {
	PrimaryRX    float64
	PrimaryRY    float64
	PrimaryGX    float64
	PrimaryGY    float64
	PrimaryBX    float64
	PrimaryBY    float64
	WhitePointX  float64
	WhitePointY  float64
	LuminanceMax float64
	LuminanceMin float64
}

// HDR10 描述一份"已确认"的 HDR10 静态元数据。
type HDR10 struct {
	Primaries uint64
	Transfer  uint64
	Matrix    uint64
	Range     uint64 // 0 表示源未声明
	HasRange  bool
	MaxCLL    uint64
	MaxFALL   uint64
	Mastering MasteringMetadata
}

// Colour 是源容器中解析出的原始 Colour 字段集合(用于确认 HDR10)。
type Colour struct {
	Matrix          uint64
	HasMatrix       bool
	Range           uint64
	HasRange        bool
	Transfer        uint64
	HasTransfer     bool
	Primaries       uint64
	HasPrimaries    bool
	MaxCLL          uint64
	HasMaxCLL       bool
	MaxFALL         uint64
	HasMaxFALL      bool
	Mastering       MasteringMetadata
	MasteringFields int // 已解析到的 mastering 子字段个数
}

// ConfirmHDR10 校验 Colour 是否构成一份"已确认"的 HDR10 元数据:
// BT.2020 三要素齐全、PQ 传输曲线、ST 2086 mastering 全字段、MaxCLL/MaxFALL 存在。
// 不满足时返回具体原因。
func (c *Colour) ConfirmHDR10() (*HDR10, error) {
	if !c.HasPrimaries || c.Primaries != PrimariesBT2020 {
		return nil, fmt.Errorf("Primaries 不是 BT.2020")
	}
	if !c.HasTransfer || c.Transfer != TransferPQ {
		return nil, fmt.Errorf("TransferCharacteristics 不是 PQ (SMPTE ST 2084)")
	}
	if !c.HasMatrix || (c.Matrix != MatrixBT2020NCL && c.Matrix != MatrixBT2020CL) {
		return nil, fmt.Errorf("MatrixCoefficients 不是 BT.2020")
	}
	if c.MasteringFields != masteringFieldCount {
		return nil, fmt.Errorf("MasteringMetadata 字段不完整 (%d/%d)", c.MasteringFields, masteringFieldCount)
	}
	if !c.HasMaxCLL || !c.HasMaxFALL {
		return nil, fmt.Errorf("缺少 MaxCLL/MaxFALL 内容亮度信息")
	}
	h := &HDR10{
		Primaries: c.Primaries,
		Transfer:  c.Transfer,
		Matrix:    c.Matrix,
		MaxCLL:    c.MaxCLL,
		MaxFALL:   c.MaxFALL,
		Mastering: c.Mastering,
	}
	if c.HasRange {
		h.Range = c.Range
		h.HasRange = true
	}
	return h, nil
}
