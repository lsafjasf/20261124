package audit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"hdr10audit/hdr"
	"hdr10audit/mp4"
)

type FixResult struct {
	Path           string   `json:"path"`
	Changed        bool     `json:"changed"`
	Patches        int      `json:"patches"`
	MediaUnchanged bool     `json:"media_payload_unchanged"`
	BeforeMediaSHA string   `json:"before_media_sha256,omitempty"`
	AfterMediaSHA  string   `json:"after_media_sha256,omitempty"`
	Notes          []string `json:"notes,omitempty"`
}

type patch struct {
	offset int64
	data   []byte
	field  string
}

// FixContainerMetadata copies path to output and rewrites only the existing
// container mdcv/clli payloads. It never adds or removes boxes, so all media
// sample offsets and mdat payload bytes remain unchanged.
func FixContainerMetadata(path, output string) (*FixResult, error) {
	if output == "" {
		output = path
	}
	src, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return nil, err
	}
	info, err := mp4.NewFile(src, st.Size()).Inspect()
	if err != nil {
		return nil, err
	}
	beforeHash, err := hashRanges(src, info.MediaData)
	if err != nil {
		return nil, err
	}
	result := &FixResult{Path: output, BeforeMediaSHA: beforeHash}
	patches, notes, err := planPatches(src, info)
	if err != nil {
		return nil, err
	}
	result.Notes = append(result.Notes, notes...)
	result.Changed = len(patches) > 0
	if !result.Changed {
		result.AfterMediaSHA = beforeHash
		result.MediaUnchanged = true
		if output != path {
			if err := copyFile(path, output, st.Mode()); err != nil {
				return nil, err
			}
		}
		return result, nil
	}
	if err := ensureOutsideMedia(patches, info.MediaData); err != nil {
		return nil, err
	}
	tmp, err := preparePatched(path, output, patches)
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmp)
		}
	}()
	after, afterHash, err := inspectOutput(tmp)
	if err != nil {
		return nil, fmt.Errorf("verify output: %w", err)
	}
	if afterHash != beforeHash {
		return nil, fmt.Errorf("media payload changed: before=%s after=%s", beforeHash, afterHash)
	}
	if len(after.MediaData) != len(info.MediaData) {
		return nil, fmt.Errorf("media data range count changed")
	}
	for i := range after.MediaData {
		if after.MediaData[i] != info.MediaData[i] {
			return nil, fmt.Errorf("media data range %d moved", i)
		}
	}
	if err := os.Chmod(tmp, st.Mode().Perm()); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, output); err != nil {
		return nil, err
	}
	committed = true
	result.AfterMediaSHA = afterHash
	result.MediaUnchanged = true
	result.Patches = len(patches)
	return result, nil
}

func planPatches(r io.ReaderAt, info *mp4.Info) ([]patch, []string, error) {
	var patches []patch
	var notes []string
	for _, track := range info.Tracks {
		stream, _ := resolveSource(track.Bitstream, hdr.SourceBitstream)
		if stream.masteringAmbiguous || stream.lightAmbiguous {
			return nil, nil, fmt.Errorf("track %d has conflicting bitstream metadata; refusing to patch", track.ID)
		}
		foundMDCV, foundCLLI := false, false
		for _, desc := range track.Descriptions {
			for _, box := range desc.MetadataBox {
				switch box.Type {
				case "mdcv":
					foundMDCV = true
					if stream.mastering == nil {
						continue
					}
					data := hdr.EncodeMastering(*stream.mastering)
					changed, err := differs(r, box.PayloadStart, data)
					if err != nil {
						return nil, nil, err
					}
					if changed {
						patches = append(patches, patch{box.PayloadStart, data, "mdcv"})
					}
				case "clli":
					foundCLLI = true
					if stream.light == nil {
						continue
					}
					data := hdr.EncodeContentLight(*stream.light)
					changed, err := differs(r, box.PayloadStart, data)
					if err != nil {
						return nil, nil, err
					}
					if changed {
						patches = append(patches, patch{box.PayloadStart, data, "clli"})
					}
				}
			}
		}
		if stream.mastering != nil && !foundMDCV {
			notes = append(notes, fmt.Sprintf("track %d: container mdcv missing; no box added", track.ID))
		}
		if stream.light != nil && !foundCLLI {
			notes = append(notes, fmt.Sprintf("track %d: container clli missing; no box added", track.ID))
		}
		if stream.mastering == nil && stream.light == nil {
			notes = append(notes, fmt.Sprintf("track %d: no bitstream HDR10 metadata; unchanged", track.ID))
		}
	}
	return patches, notes, nil
}

func differs(r io.ReaderAt, offset int64, want []byte) (bool, error) {
	have := make([]byte, len(want))
	if _, err := r.ReadAt(have, offset); err != nil {
		return false, err
	}
	for i := range want {
		if have[i] != want[i] {
			return true, nil
		}
	}
	return false, nil
}

func ensureOutsideMedia(patches []patch, ranges []mp4.Range) error {
	for _, p := range patches {
		end := p.offset + int64(len(p.data))
		for _, rg := range ranges {
			if p.offset < rg.Start+rg.Size && end > rg.Start {
				return fmt.Errorf("%s patch [%d,%d) overlaps media payload [%d,%d)",
					p.field, p.offset, end, rg.Start, rg.Start+rg.Size)
			}
		}
	}
	return nil
}

func preparePatched(srcPath, dstPath string, patches []patch) (string, error) {
	in, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer in.Close()
	dir := filepath.Dir(dstPath)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dstPath)+".hdr10audit-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		return "", err
	}
	for _, p := range patches {
		if _, err := tmp.WriteAt(p.data, p.offset); err != nil {
			return "", fmt.Errorf("patch %s at %d: %w", p.field, p.offset, err)
		}
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	ok = true
	return tmpName, nil
}

func inspectOutput(path string) (*mp4.Info, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	info, err := mp4.NewFile(f, st.Size()).Inspect()
	if err != nil {
		return nil, "", err
	}
	hash, err := hashRanges(f, info.MediaData)
	if err != nil {
		return nil, "", err
	}
	return info, hash, nil
}

func copyFile(srcPath, dstPath string, mode os.FileMode) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
