package id3

import "errors"

// Version is an ID3v2 major/minor version pair (e.g. 3.0 or 4.0).
type Version struct {
	Major byte
	Minor byte
}

func (v Version) String() string {
	return "ID3v2." + string('0'+v.Major) + "." + string('0'+v.Minor)
}

// Frame is one raw or decoded ID3v2 frame. Unknown frames keep their wire
// payload in RawData so they can be written back byte-for-byte.
type Frame struct {
	ID           string
	Version      Version
	Flags        uint16
	GroupID      byte
	HasGroup     bool
	Encrypted    bool
	Compressed   bool
	RawData      []byte // payload as stored (still compressed/encrypted when set)
	Data         []byte // decoded payload (unsynchronisation / encoding reversed)
	SubFrames    []*Frame
	Start, End   uint32
	StOff, EnOff uint32
	ElementID    string
	ChildIDs     []string // CTOC ordered child element ids
	Title        string   // TIT2 text of the frame / subframe
	Text         string   // decoded text for T*** frames
	Order        int      // original frame order inside the tag
}

// Tag is a parsed ID3v2 tag plus the location of the audio payload.
type Tag struct {
	Version      Version
	HeaderFlags  byte
	TagSize      int // size of the whole tag block including header/footer
	BodyStart    int // first byte after header (+ extended header)
	BodyEnd      int // first byte of audio (or footer)
	HasFooter    bool
	HasExtHeader bool
	ExtHeader    []byte
	Frames       []*Frame
	Audio        []byte // bytes following the tag, never modified
}

var (
	ErrNoTag     = errors.New("id3: no ID3v2 tag found")
	ErrBadHeader = errors.New("id3: malformed tag header")
	ErrVersion   = errors.New("id3: unsupported ID3v2 version")
	ErrTruncated = errors.New("id3: truncated frame")
	ErrBadFrame  = errors.New("id3: malformed frame")
)
