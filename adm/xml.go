package adm

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// Position is one coordinate of an audioBlockFormat.
type Position struct {
	Coordinate string  `xml:"coordinate,attr"`
	Value      float64 `xml:",chardata"`
}

// BlockFormat is an audioBlockFormat with optional time interval.
type BlockFormat struct {
	ID        string     `xml:"audioBlockFormatID,attr"`
	RTime     string     `xml:"rtime,attr"`
	Duration  string     `xml:"duration,attr"`
	Positions []Position `xml:"position"`
}

// ChannelFormat is an audioChannelFormat (bed channel or object channel).
type ChannelFormat struct {
	ID           string        `xml:"audioChannelFormatID,attr"`
	Name         string        `xml:"audioChannelFormatName,attr"`
	TypeLabel    string        `xml:"typeLabel,attr"`
	TypeDef      string        `xml:"typeDefinition,attr"`
	BlockFormats []BlockFormat `xml:"audioBlockFormat"`
}

// PackFormat is an audioPackFormat (channel group / bed grouping).
type PackFormat struct {
	ID            string   `xml:"audioPackFormatID,attr"`
	Name          string   `xml:"audioPackFormatName,attr"`
	TypeLabel     string   `xml:"typeLabel,attr"`
	TypeDef       string   `xml:"typeDefinition,attr"`
	ChannelRefIDs []string `xml:"audioChannelFormatIDRef"`
	PackRefIDs    []string `xml:"audioPackFormatIDRef"`
}

// StreamFormat links a track format to a channel/pack format.
type StreamFormat struct {
	ID           string `xml:"audioStreamFormatID,attr"`
	Name         string `xml:"audioStreamFormatName,attr"`
	ChannelRefID string `xml:"audioChannelFormatIDRef"`
	PackRefID    string `xml:"audioPackFormatIDRef"`
	TrackRefID   string `xml:"audioTrackFormatIDRef"`
}

// TrackFormat is an audioTrackFormat.
type TrackFormat struct {
	ID          string `xml:"audioTrackFormatID,attr"`
	Name        string `xml:"audioTrackFormatName,attr"`
	StreamRefID string `xml:"audioStreamFormatIDRef"`
}

// TrackUID is an audioTrackUID element: the media reference that ties an
// ADM definition to one physical track of the BWF.
type TrackUID struct {
	UID        string `xml:"UID,attr"`
	SampleRate string `xml:"sampleRate,attr"`
	BitDepth   string `xml:"bitDepth,attr"`
	TrackRefID string `xml:"audioTrackFormatIDRef"`
	PackRefID  string `xml:"audioPackFormatIDRef"`
}

// AudioObject is an audioObject (bed or object instance on the timeline).
type AudioObject struct {
	ID           string   `xml:"audioObjectID,attr"`
	Name         string   `xml:"audioObjectName,attr"`
	Start        string   `xml:"start,attr"`
	Duration     string   `xml:"duration,attr"`
	PackRefIDs   []string `xml:"audioPackFormatIDRef"`
	TrackUIDRefs []string `xml:"audioTrackUIDRef"`
	ObjectRefIDs []string `xml:"audioObjectIDRef"`
}

// AudioContent groups objects.
type AudioContent struct {
	ID           string   `xml:"audioContentID,attr"`
	Name         string   `xml:"audioContentName,attr"`
	ObjectRefIDs []string `xml:"audioObjectIDRef"`
}

// AudioProgramme is the top-level editorial element.
type AudioProgramme struct {
	ID            string   `xml:"audioProgrammeID,attr"`
	Name          string   `xml:"audioProgrammeName,attr"`
	Start         string   `xml:"start,attr"`
	End           string   `xml:"end,attr"`
	ContentRefIDs []string `xml:"audioContentIDRef"`
}

// Document is the subset of audioFormatExtended we verify.
type Document struct {
	Programmes []AudioProgramme
	Contents   []AudioContent
	Objects    []AudioObject
	Packs      []PackFormat
	Channels   []ChannelFormat
	Streams    []StreamFormat
	Tracks     []TrackFormat
	TrackUIDs  []TrackUID
}

type afeXML struct {
	Programmes []AudioProgramme `xml:"audioProgramme"`
	Contents   []AudioContent   `xml:"audioContent"`
	Objects    []AudioObject    `xml:"audioObject"`
	Packs      []PackFormat     `xml:"audioPackFormat"`
	Channels   []ChannelFormat  `xml:"audioChannelFormat"`
	Streams    []StreamFormat   `xml:"audioStreamFormat"`
	Tracks     []TrackFormat    `xml:"audioTrackFormat"`
	TrackUIDs  []TrackUID       `xml:"audioTrackUID"`
}

// ParseXML decodes an axml chunk payload. Namespaces are ignored so both
// plain audioFormatExtended and ebuCoreMain-wrapped documents parse.
func ParseXML(b []byte) (*Document, error) {
	dec := xml.NewDecoder(strings.NewReader(string(b)))
	var doc Document
	found := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decoding axml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "audioFormatExtended" {
			continue
		}
		var afe afeXML
		if err := dec.DecodeElement(&afe, &se); err != nil {
			return nil, fmt.Errorf("decoding audioFormatExtended: %w", err)
		}
		doc.Programmes = append(doc.Programmes, afe.Programmes...)
		doc.Contents = append(doc.Contents, afe.Contents...)
		doc.Objects = append(doc.Objects, afe.Objects...)
		doc.Packs = append(doc.Packs, afe.Packs...)
		doc.Channels = append(doc.Channels, afe.Channels...)
		doc.Streams = append(doc.Streams, afe.Streams...)
		doc.Tracks = append(doc.Tracks, afe.Tracks...)
		doc.TrackUIDs = append(doc.TrackUIDs, afe.TrackUIDs...)
		found = true
	}
	if !found {
		return nil, fmt.Errorf("no audioFormatExtended element found in axml")
	}
	return &doc, nil
}
