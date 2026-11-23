package relink

import (
	"context"
	"errors"
	"fmt"
)

const ProjectVersion = 1

type FrameRate struct {
	Numerator   int64 `json:"numerator"`
	Denominator int64 `json:"denominator"`
}

func (r FrameRate) String() string {
	if r.Denominator == 0 {
		return "invalid"
	}
	return fmt.Sprintf("%d/%d", r.Numerator, r.Denominator)
}

func (r FrameRate) Valid() bool {
	return r.Numerator > 0 && r.Denominator > 0
}

func (r FrameRate) Float() float64 {
	if r.Denominator == 0 {
		return 0
	}
	return float64(r.Numerator) / float64(r.Denominator)
}

func (r FrameRate) CloseTo(other FrameRate, tolerance float64) bool {
	if !r.Valid() || !other.Valid() {
		return false
	}
	delta := r.Float() - other.Float()
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}

// Fingerprint is both the expected technical metadata stored in a project and
// the metadata returned by a Probe for an actual media file.
type Fingerprint struct {
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256,omitempty"`
	DurationMS    int64     `json:"duration_ms"`
	FrameRate     FrameRate `json:"frame_rate"`
	AudioOffsetMS int64     `json:"audio_offset_ms"`
}

type MediaRef struct {
	Path        string      `json:"path"`
	Fingerprint Fingerprint `json:"fingerprint"`
}

type Clip struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Primary MediaRef  `json:"primary"`
	Proxy   *MediaRef `json:"proxy,omitempty"`
}

type TimelineItem struct {
	ID            string `json:"id"`
	ClipID        string `json:"clip_id"`
	StartFrame    int64  `json:"start_frame"`
	EndFrame      int64  `json:"end_frame"`
	SourceInFrame int64  `json:"source_in_frame"`
	AudioOffsetMS int64  `json:"audio_offset_ms"`
}

type Project struct {
	Version       int            `json:"version"`
	Name          string         `json:"name,omitempty"`
	FrameRate     FrameRate      `json:"frame_rate"`
	DurationMS    int64          `json:"duration_ms"`
	AudioOffsetMS int64          `json:"audio_offset_ms"`
	Timeline      []TimelineItem `json:"timeline"`
	Clips         []Clip         `json:"clips"`
}

type Confidence string

const (
	ConfidenceHigh Confidence = "high"
	ConfidenceLow  Confidence = "low"
)

type Role string

const (
	RolePrimary Role = "primary"
	RoleProxy   Role = "proxy"
)

type Candidate struct {
	Path    string
	RelPath string
	Info    Fingerprint
	Score   int
}

type Proposal struct {
	ClipID       string
	ClipName     string
	Role         Role
	OldPath      string
	Selected     Candidate
	Alternatives []Candidate
	Confidence   Confidence
	Reason       string
}

type Approval struct {
	Accept        bool
	CandidatePath string
}

// Approver receives one batch call for all high-confidence proposals and one
// call per low-confidence proposal. Declining any item aborts the transaction.
type Approver interface {
	ApproveBatch(context.Context, []Proposal) (bool, error)
	ApproveOne(context.Context, Proposal) (Approval, error)
}

type MissingProxyPolicy string

const (
	MissingProxyFail MissingProxyPolicy = "fail"
	MissingProxyKeep MissingProxyPolicy = "keep"
)

type Link struct {
	ClipID     string     `json:"clip_id"`
	Role       Role       `json:"role"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Confidence Confidence `json:"confidence"`
}

type Warning struct {
	Code    string `json:"code"`
	ClipID  string `json:"clip_id,omitempty"`
	Role    Role   `json:"role,omitempty"`
	Message string `json:"message"`
}

type Check struct {
	ClipID        string `json:"clip_id"`
	Role          Role   `json:"role"`
	FrameRateOK   bool   `json:"frame_rate_ok"`
	DurationOK    bool   `json:"duration_ok"`
	AudioOffsetOK bool   `json:"audio_offset_ok"`
}

type Report struct {
	ProjectPath   string    `json:"project_path"`
	BatchAccepted int       `json:"batch_accepted"`
	LowAccepted   int       `json:"low_accepted"`
	Links         []Link    `json:"links"`
	Checks        []Check   `json:"checks"`
	Warnings      []Warning `json:"warnings"`
}

var (
	ErrNotConfirmed     = errors.New("relink: confirmation declined")
	ErrNoCandidates     = errors.New("relink: no valid candidate")
	ErrCandidateInvalid = errors.New("relink: candidate failed media validation")
)
