package tagfix

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Option configures ApplyFile.
type Option func(*applyConfig)

type applyConfig struct {
	track int
}

// WithTrack selects a zero-based AVC video track. The default is the first.
func WithTrack(index int) Option {
	return func(config *applyConfig) {
		config.track = index
	}
}

var afterReplaceHook func() error

// ApplyFile synchronizes tags in place. The original inode is preserved through
// a hard link until the replacement has been parsed back successfully.
func ApplyFile(path string, tags ColorTags, options ...Option) (*Report, error) {
	config := applyConfig{}
	for _, option := range options {
		option(&config)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return &Report{File: path, Status: "rejected", Reason: err.Error()}, err
	}
	if !info.Mode().IsRegular() {
		err := fmt.Errorf("%w: %s is not a regular file", ErrWriteFailed, path)
		return &Report{File: path, Status: "rejected", Reason: err.Error()}, err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return &Report{File: path, Status: "rejected", Reason: err.Error()}, err
	}
	newData, report, err := BuildTrack(original, tags, config.track)
	report.File = path
	if err != nil {
		verifyOriginal(path, original, report)
		return report, err
	}
	if bytes.Equal(original, newData) {
		report.Status = "unchanged"
		verifyOriginal(path, original, report)
		return report, nil
	}
	if err := replaceAtomically(path, info.Mode().Perm(), original, newData, report); err != nil {
		if report.Status == "rolled_back" {
			refreshRollbackChanges(path, report, original, tags, config.track)
		}
		return report, err
	}
	report.Status = "applied"
	verifyOriginal(path, newData, report)
	return report, nil
}

func replaceAtomically(path string, mode os.FileMode, original, newData []byte, report *Report) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	temp, err := os.CreateTemp(dir, "."+base+".tagfix-new-*")
	if err != nil {
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: create temporary file: %v", ErrWriteFailed, err))
	}
	tempPath := temp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: preserve mode: %v", ErrWriteFailed, err))
	}
	if err := writeAll(temp, newData); err != nil {
		_ = temp.Close()
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: write temporary file: %v", ErrWriteFailed, err))
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: sync temporary file: %v", ErrWriteFailed, err))
	}
	if err := temp.Close(); err != nil {
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: close temporary file: %v", ErrWriteFailed, err))
	}
	written, err := os.ReadFile(tempPath)
	if err != nil || !bytes.Equal(written, newData) {
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: temporary readback does not match planned bytes", ErrWriteFailed))
	}

	backupPath, err := makeBackup(path, dir, base)
	if err != nil {
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: create backup link: %v", ErrWriteFailed, err))
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(backupPath)
		return preReplaceFailure(path, original, report,
			fmt.Errorf("%w: replace target: %v", ErrWriteFailed, err))
	}
	cleanupTemp = false

	if err := syncDir(dir); err != nil {
		return rollbackReplacement(path, backupPath, original, report,
			fmt.Errorf("%w: sync directory: %v", ErrWriteFailed, err))
	}
	if afterReplaceHook != nil {
		if err := afterReplaceHook(); err != nil {
			return rollbackReplacement(path, backupPath, original, report,
				fmt.Errorf("%w: post-replace check: %v", ErrWriteFailed, err))
		}
	}
	if err := verifyFile(path, newData, report, false); err != nil {
		return rollbackReplacement(path, backupPath, original, report, err)
	}
	if err := os.Remove(backupPath); err != nil {
		report.Rollback.Detail = "replacement verified; backup cleanup failed: " + err.Error()
	}
	_ = syncDir(dir)
	return nil
}

func makeBackup(path, dir, base string) (string, error) {
	file, err := os.CreateTemp(dir, "."+base+".tagfix-backup-*")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	if err := os.Link(path, name); err != nil {
		return "", err
	}
	if err := syncDir(dir); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func preReplaceFailure(path string, original []byte, report *Report, err error) error {
	report.Status = "rolled_back"
	report.Reason = err.Error()
	report.Rollback.Required = true
	report.Rollback.Performed = false
	report.Rollback.Verified = verifyPathEquals(path, original)
	if !report.Rollback.Verified {
		report.Rollback.Detail = "original bytes could not be verified after pre-replace failure"
	}
	return err
}

func rollbackReplacement(path, backupPath string, original []byte, report *Report, cause error) error {
	report.Status = "rolled_back"
	report.Reason = cause.Error()
	report.Rollback.Required = true
	report.Rollback.Performed = true
	if err := os.Rename(backupPath, path); err != nil {
		report.Rollback.Verified = false
		report.Rollback.Detail = "restore original failed: " + err.Error()
		return fmt.Errorf("%w: restore %s from %s: %v",
			ErrRollbackFailed, path, backupPath, err)
	}
	_ = syncDir(filepath.Dir(path))
	report.Rollback.Verified = verifyPathEquals(path, original)
	if !report.Rollback.Verified {
		report.Rollback.Detail = "restored file does not match the original digest"
		return fmt.Errorf("%w: restored file does not match original", ErrRollbackFailed)
	}
	return cause
}

func verifyOriginal(path string, original []byte, report *Report) {
	report.Rollback.Verified = verifyPathEquals(path, original)
	if !report.Rollback.Verified {
		report.Rollback.Detail = "on-disk file differs from expected bytes"
	}
}

func verifyFile(path string, expected []byte, report *Report, updateChanges bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: read replaced file: %v", ErrReadbackMismatch, err)
	}
	if !bytes.Equal(data, expected) {
		return fmt.Errorf("%w: replaced file bytes differ", ErrReadbackMismatch)
	}
	current, err := analyze(data, report.Track)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReadbackMismatch, err)
	}
	if updateChanges {
		_ = current
	}
	return nil
}

func refreshRollbackChanges(path string, report *Report, original []byte, tags ColorTags, track int) {
	old, err := analyze(original, track)
	if err != nil {
		return
	}
	actualData, readErr := os.ReadFile(path)
	actual := old
	if readErr == nil {
		if parsed, parseErr := analyze(actualData, track); parseErr == nil {
			actual = parsed
		}
	}
	report.Changes = diffChanges(old, actual, tags)
	report.Payload = mergePayload(old.mdat, actual.mdat)
}

func verifyPathEquals(path string, expected []byte) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	expectedDigest := sha256.Sum256(expected)
	actualDigest := sha256.Sum256(data)
	return expectedDigest == actualDigest && bytes.Equal(data, expected)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func syncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
