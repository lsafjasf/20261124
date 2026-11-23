package tagfix

import (
	"fmt"
)

const (
	fieldPrimaries = "colour_primaries"
	fieldTransfer  = "transfer_characteristics"
	fieldMatrix    = "matrix_coefficients"
	fieldFullRange = "video_full_range"
)

type fieldValue struct {
	bit     uint64
	value   any
	present bool
}

type spsInfo struct {
	nal            []byte
	rbsp           []byte
	rbspToEBSP     []int
	baseOffset     int64
	profileIDC     uint8
	hasVUI         bool
	hasSignal      bool
	hasDescription bool
	vuiFlagPos     uint64
	signalFlagPos  uint64
	fullRangePos   uint64
	descFlagPos    uint64
	primariesPos   uint64
	transferPos    uint64
	matrixPos      uint64
	fields         map[string]fieldValue
}

func highProfile(idc uint8) bool {
	switch idc {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	default:
		return false
	}
}

func parseSPS(nal []byte, baseOffset int64) (*spsInfo, error) {
	if err := checkNALHeader(nal, 7); err != nil {
		return nil, err
	}
	rbsp, mapping := removeEmulationPrevention(nal[1:])
	reader := &bitReader{data: rbsp}
	profile, err := reader.readBits(8)
	if err != nil {
		return nil, err
	}
	if _, err := reader.readBits(8); err != nil {
		return nil, err
	}
	if _, err := reader.readBits(8); err != nil {
		return nil, err
	}
	if _, err := reader.readUE(); err != nil {
		return nil, err
	}

	info := &spsInfo{
		nal:        append([]byte(nil), nal...),
		rbsp:       rbsp,
		rbspToEBSP: mapping,
		baseOffset: baseOffset,
		profileIDC: uint8(profile),
		fields:     make(map[string]fieldValue, 4),
	}
	if highProfile(info.profileIDC) {
		if err := skipHighProfileFields(reader); err != nil {
			return nil, err
		}
	}
	if _, err := reader.readUE(); err != nil { // log2_max_frame_num_minus4
		return nil, err
	}
	pocType, err := reader.readUE()
	if err != nil {
		return nil, err
	}
	switch pocType {
	case 0:
		if _, err := reader.readUE(); err != nil { // log2_max_pic_order_cnt_lsb_minus4
			return nil, err
		}
	case 1:
		if _, err := reader.readBit(); err != nil {
			return nil, err
		}
		if _, err := reader.readSE(); err != nil {
			return nil, err
		}
		if _, err := reader.readSE(); err != nil {
			return nil, err
		}
		cycle, err := reader.readUE()
		if err != nil {
			return nil, err
		}
		if cycle > 255 {
			return nil, fmt.Errorf("%w: implausible POC cycle length %d",
				ErrUnsupportedMedia, cycle)
		}
		for i := uint64(0); i < cycle; i++ {
			if _, err := reader.readSE(); err != nil {
				return nil, err
			}
		}
	}
	if _, err := reader.readUE(); err != nil { // max_num_ref_frames
		return nil, err
	}
	if _, err := reader.readBit(); err != nil { // gaps_in_frame_num_value_allowed_flag
		return nil, err
	}
	if _, err := reader.readUE(); err != nil { // pic_width_in_mbs_minus1
		return nil, err
	}
	if _, err := reader.readUE(); err != nil { // pic_height_in_map_units_minus1
		return nil, err
	}
	frameMBSOnly, err := reader.readBit()
	if err != nil {
		return nil, err
	}
	if frameMBSOnly == 0 {
		if _, err := reader.readBit(); err != nil {
			return nil, err
		}
	}
	if _, err := reader.readBit(); err != nil { // direct_8x8_inference_flag
		return nil, err
	}
	cropping, err := reader.readBit()
	if err != nil {
		return nil, err
	}
	if cropping == 1 {
		for i := 0; i < 4; i++ {
			if _, err := reader.readUE(); err != nil {
				return nil, err
			}
		}
	}

	info.vuiFlagPos = reader.pos
	vui, err := reader.readBit()
	if err != nil {
		return nil, err
	}
	info.hasVUI = vui == 1
	if info.hasVUI {
		if err := info.parseVUI(reader); err != nil {
			return nil, err
		}
	}
	return info, nil
}

func skipHighProfileFields(reader *bitReader) error {
	chroma, err := reader.readUE()
	if err != nil {
		return err
	}
	if chroma == 3 {
		if _, err := reader.readBit(); err != nil {
			return err
		}
	}
	if _, err := reader.readUE(); err != nil { // bit_depth_luma_minus8
		return err
	}
	if _, err := reader.readUE(); err != nil { // bit_depth_chroma_minus8
		return err
	}
	if _, err := reader.readBit(); err != nil { // qpprime_y_zero_transform_bypass_flag
		return err
	}
	scaling, err := reader.readBit()
	if err != nil {
		return err
	}
	if scaling == 0 {
		return nil
	}
	count := 8
	if chroma == 3 {
		count = 12
	}
	for i := 0; i < count; i++ {
		present, err := reader.readBit()
		if err != nil {
			return err
		}
		if present == 0 {
			continue
		}
		size := 16
		if i >= 6 {
			size = 64
		}
		if err := skipScalingList(reader, size); err != nil {
			return err
		}
	}
	return nil
}

func skipScalingList(reader *bitReader, size int) error {
	lastScale := int64(8)
	nextScale := int64(8)
	for i := 0; i < size; i++ {
		if nextScale != 0 {
			delta, err := reader.readSE()
			if err != nil {
				return err
			}
			nextScale = (lastScale + delta + 256) % 256
		}
		if nextScale != 0 {
			lastScale = nextScale
		}
	}
	return nil
}

func (s *spsInfo) parseVUI(reader *bitReader) error {
	aspect, err := reader.readBit()
	if err != nil {
		return err
	}
	if aspect == 1 {
		idc, err := reader.readBits(8)
		if err != nil {
			return err
		}
		if idc == 255 {
			if _, err := reader.readBits(16); err != nil {
				return err
			}
			if _, err := reader.readBits(16); err != nil {
				return err
			}
		}
	}
	overscan, err := reader.readBit()
	if err != nil {
		return err
	}
	if overscan == 1 {
		if _, err := reader.readBit(); err != nil {
			return err
		}
	}

	s.signalFlagPos = reader.pos
	signal, err := reader.readBit()
	if err != nil {
		return err
	}
	s.hasSignal = signal == 1
	if !s.hasSignal {
		return nil
	}
	if _, err := reader.readBits(3); err != nil { // video_format
		return err
	}
	s.fullRangePos = reader.pos
	fullRange, err := reader.readBit()
	if err != nil {
		return err
	}
	s.descFlagPos = reader.pos
	description, err := reader.readBit()
	if err != nil {
		return err
	}
	s.hasDescription = description == 1
	s.fields[fieldFullRange] = fieldValue{
		bit: s.fullRangePos, value: fullRange == 1, present: true,
	}
	if !s.hasDescription {
		return nil
	}
	s.primariesPos = reader.pos
	primaries, err := reader.readBits(8)
	if err != nil {
		return err
	}
	s.transferPos = reader.pos
	transfer, err := reader.readBits(8)
	if err != nil {
		return err
	}
	s.matrixPos = reader.pos
	matrix, err := reader.readBits(8)
	if err != nil {
		return err
	}
	s.fields[fieldPrimaries] = fieldValue{
		bit: s.primariesPos, value: uint8(primaries), present: true,
	}
	s.fields[fieldTransfer] = fieldValue{
		bit: s.transferPos, value: uint8(transfer), present: true,
	}
	s.fields[fieldMatrix] = fieldValue{
		bit: s.matrixPos, value: uint8(matrix), present: true,
	}
	return nil
}

func (s *spsInfo) fieldOffset(name string) (int64, int, bool) {
	field, ok := s.fields[name]
	if !ok || !field.present {
		return 0, 0, false
	}
	byteIndex := field.bit / 8
	if byteIndex >= uint64(len(s.rbspToEBSP)) {
		return 0, 0, false
	}
	offset := s.baseOffset + 1 + int64(s.rbspToEBSP[byteIndex])
	return offset, int(field.bit % 8), true
}

func (s *spsInfo) tags() (ColorTags, bool) {
	primaries, ok1 := s.fields[fieldPrimaries]
	transfer, ok2 := s.fields[fieldTransfer]
	matrix, ok3 := s.fields[fieldMatrix]
	fullRange, ok4 := s.fields[fieldFullRange]
	if !ok1 || !ok2 || !ok3 || !ok4 ||
		!primaries.present || !transfer.present || !matrix.present || !fullRange.present {
		return ColorTags{}, false
	}
	return ColorTags{
		Primaries: primaries.value.(uint8),
		Transfer:  transfer.value.(uint8),
		Matrix:    matrix.value.(uint8),
		FullRange: fullRange.value.(bool),
	}, true
}

func (s *spsInfo) patch(tags ColorTags) ([]byte, error) {
	bits := bytesToBits(s.rbsp)
	var err error
	switch {
	case !s.hasVUI:
		err = setBit(bits, s.vuiFlagPos, 1)
		inserted := []byte{0, 0, 1} // aspect, overscan, video_signal_type
		inserted = appendBits(inserted, 5, 3)
		inserted = appendRangeAndColors(inserted, tags)
		inserted = append(inserted, 0, 0, 0, 0, 0, 0)
		if err == nil {
			bits, err = insertBits(bits, s.vuiFlagPos+1, inserted)
		}
	case !s.hasSignal:
		err = setBit(bits, s.signalFlagPos, 1)
		inserted := appendBits(nil, 5, 3)
		inserted = appendRangeAndColors(inserted, tags)
		if err == nil {
			bits, err = insertBits(bits, s.signalFlagPos+1, inserted)
		}
	case !s.hasDescription:
		rangeBit := byte(0)
		if tags.FullRange {
			rangeBit = 1
		}
		err = setBit(bits, s.fullRangePos, rangeBit)
		if err == nil {
			err = setBit(bits, s.descFlagPos, 1)
		}
		inserted := appendBits(nil, uint64(tags.Primaries), 8)
		inserted = appendBits(inserted, uint64(tags.Transfer), 8)
		inserted = appendBits(inserted, uint64(tags.Matrix), 8)
		if err == nil {
			bits, err = insertBits(bits, s.descFlagPos+1, inserted)
		}
	default:
		rangeBit := byte(0)
		if tags.FullRange {
			rangeBit = 1
		}
		for _, change := range []struct {
			pos   uint64
			value uint64
			count int
		}{
			{s.fullRangePos, uint64(rangeBit), 1},
			{s.primariesPos, uint64(tags.Primaries), 8},
			{s.transferPos, uint64(tags.Transfer), 8},
			{s.matrixPos, uint64(tags.Matrix), 8},
		} {
			for i := 0; i < change.count; i++ {
				pos := change.pos + uint64(i)
				value := byte((change.value >> (change.count - 1 - i)) & 1)
				if bitErr := setBit(bits, pos, value); bitErr != nil {
					return nil, bitErr
				}
			}
		}
	}
	if err != nil {
		return nil, err
	}
	payload := insertEmulationPrevention(bitsToBytes(trimRBSP(bits)))
	out := make([]byte, 1+len(payload))
	out[0] = s.nal[0]
	copy(out[1:], payload)
	return out, nil
}

func appendRangeAndColors(dst []byte, tags ColorTags) []byte {
	if tags.FullRange {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}
	dst = append(dst, 1) // colour_description_present_flag
	dst = appendBits(dst, uint64(tags.Primaries), 8)
	dst = appendBits(dst, uint64(tags.Transfer), 8)
	dst = appendBits(dst, uint64(tags.Matrix), 8)
	return dst
}
