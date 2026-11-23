package mkv

import (
	"io"

	"hdr10remux/internal/ebml"
)

// masterIDs 是需要递归进入的主元素。
var masterIDs = map[uint32]bool{
	IDEBMLHeader:    true,
	IDSegment:       true,
	IDInfo:          true,
	IDTracks:        true,
	IDTrackEntry:    true,
	IDVideo:         true,
	IDAudio:         true,
	IDColour:        true,
	IDMasteringMeta: true,
	IDCluster:       true,
	IDBlockGroup:    true,
}

// WalkElements 结构化遍历整个文件,对每个元素(含叶子)调用 visit。
// 未知主元素按叶子处理(跳过其负载)。
func WalkElements(r io.ReadSeeker, visit func(id uint32)) error {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return err
	}
	end, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return walkRange(r, end, visit)
}

func walkRange(r io.ReadSeeker, end int64, visit func(id uint32)) error {
	for {
		pos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		if pos >= end {
			return nil
		}
		id, size, hdrLen, err := ebml.ReadElementHeader(r)
		if err != nil {
			return err
		}
		visit(id)
		dataEnd := end
		if size != ebml.UnknownSize {
			dataEnd = pos + hdrLen + size
		}
		if masterIDs[id] {
			if err := walkRange(r, dataEnd, visit); err != nil {
				return err
			}
		}
		if _, err := r.Seek(dataEnd, io.SeekStart); err != nil {
			return err
		}
	}
}
