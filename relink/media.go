package relink

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const MetadataSuffix = ".media.json"

var ErrMetadataUnavailable = errors.New("relink: media timing metadata is unavailable")

type Probe interface {
	Probe(path string) (Fingerprint, error)
}

type ProbeFunc func(path string) (Fingerprint, error)

func (fn ProbeFunc) Probe(path string) (Fingerprint, error) {
	return fn(path)
}

// FilesystemProbe computes size and SHA-256 from the file, then reads timing
// metadata from "<path>.media.json". Native format decoders can implement Probe
// without changing the transaction or matching code.
type FilesystemProbe struct{}

type timingSidecar struct {
	DurationMS    int64     `json:"duration_ms"`
	FrameRate     FrameRate `json:"frame_rate"`
	AudioOffsetMS int64     `json:"audio_offset_ms"`
}

func (FilesystemProbe) Probe(path string) (Fingerprint, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Fingerprint{}, fmt.Errorf("stat media: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Fingerprint{}, fmt.Errorf("media is not a regular file")
	}
	digest, err := fileSHA256(path)
	if err != nil {
		return Fingerprint{}, err
	}

	data, err := os.ReadFile(path + MetadataSuffix)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Fingerprint{}, fmt.Errorf("%w for %q", ErrMetadataUnavailable, path)
		}
		return Fingerprint{}, fmt.Errorf("read media metadata: %w", err)
	}
	var sidecar timingSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return Fingerprint{}, fmt.Errorf("parse media metadata for %q: %w", path, err)
	}
	if sidecar.DurationMS <= 0 {
		return Fingerprint{}, fmt.Errorf("media metadata for %q has a non-positive duration", path)
	}
	if !sidecar.FrameRate.Valid() {
		return Fingerprint{}, fmt.Errorf("media metadata for %q has an invalid frame rate", path)
	}
	return Fingerprint{
		Size:          info.Size(),
		SHA256:        digest,
		DurationMS:    sidecar.DurationMS,
		FrameRate:     sidecar.FrameRate,
		AudioOffsetMS: sidecar.AudioOffsetMS,
	}, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open media: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash media: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type tolerances struct {
	durationMS int64
	frameRate  float64
	audioMS    int64
}

type ValidationError struct {
	ClipID string
	Role   Role
	Issues []string
}

func (err *ValidationError) Error() string {
	if err.ClipID == "" {
		return fmt.Sprintf("media validation failed: %s", strings.Join(err.Issues, "; "))
	}
	return fmt.Sprintf("clip %q %s validation failed: %s",
		err.ClipID, err.Role, strings.Join(err.Issues, "; "))
}

func compareFingerprint(expected, actual Fingerprint, tol tolerances) (Check, error) {
	var issues []string
	check := Check{
		FrameRateOK:   expected.FrameRate.CloseTo(actual.FrameRate, tol.frameRate),
		DurationOK:    closeDuration(expected.DurationMS, actual.DurationMS, tol.durationMS),
		AudioOffsetOK: closeDuration(expected.AudioOffsetMS, actual.AudioOffsetMS, tol.audioMS),
	}
	if !check.FrameRateOK {
		issues = append(issues, fmt.Sprintf("frame rate expected %s, got %s",
			expected.FrameRate, actual.FrameRate))
	}
	if !check.DurationOK {
		issues = append(issues, fmt.Sprintf("duration expected %dms, got %dms",
			expected.DurationMS, actual.DurationMS))
	}
	if !check.AudioOffsetOK {
		issues = append(issues, fmt.Sprintf("audio offset expected %dms, got %dms",
			expected.AudioOffsetMS, actual.AudioOffsetMS))
	}
	if expected.Size > 0 && expected.Size != actual.Size {
		issues = append(issues, fmt.Sprintf("size expected %d, got %d",
			expected.Size, actual.Size))
	}
	if expected.SHA256 != "" && !strings.EqualFold(expected.SHA256, actual.SHA256) {
		issues = append(issues, "SHA-256 mismatch")
	}
	if len(issues) > 0 {
		return check, &ValidationError{Issues: issues}
	}
	return check, nil
}

func closeDuration(want, got, tolerance int64) bool {
	delta := want - got
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}
