package adm

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse reads an ADM XML document (the axml chunk payload or a plain .xml file).
func Parse(r io.Reader) (*Document, error) {
	var root xmlNode
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("parsing ADM XML: %w", err)
	}
	doc := &Document{
		ChannelFormats: map[string]*ChannelFormat{},
		PackFormats:    map[string]*PackFormat{},
		TrackUIDs:      map[string]*TrackUID{},
	}
	afe := findDeep(&root, "audioFormatExtended")
	if afe == nil {
		return nil, fmt.Errorf("no audioFormatExtended element found")
	}
	for i := range afe.Children {
		n := &afe.Children[i]
		switch n.XMLName.Local {
		case "audioObject":
			doc.Objects = append(doc.Objects, parseObject(n))
		case "audioChannelFormat":
			cf := parseChannelFormat(n, doc)
			doc.ChannelFormats[cf.ID] = cf
		case "audioPackFormat":
			pf := parsePackFormat(n)
			doc.PackFormats[pf.ID] = pf
		case "audioTrackUID":
			tu := parseTrackUID(n)
			doc.TrackUIDs[tu.ID] = tu
		}
	}
	return doc, nil
}

type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xmlNode  `xml:",any"`
	Text     string     `xml:",chardata"`
}

func (n *xmlNode) attr(name string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n *xmlNode) child(name string) *xmlNode {
	for i := range n.Children {
		if n.Children[i].XMLName.Local == name {
			return &n.Children[i]
		}
	}
	return nil
}

func (n *xmlNode) children(name string) []*xmlNode {
	var out []*xmlNode
	for i := range n.Children {
		if n.Children[i].XMLName.Local == name {
			out = append(out, &n.Children[i])
		}
	}
	return out
}

func findDeep(n *xmlNode, name string) *xmlNode {
	if n.XMLName.Local == name {
		return n
	}
	for i := range n.Children {
		if f := findDeep(&n.Children[i], name); f != nil {
			return f
		}
	}
	return nil
}

func parseObject(n *xmlNode) Object {
	o := Object{
		ID:       n.attr("audioObjectID"),
		Name:     n.attr("audioObjectName"),
		Gain:     1,
		Dialogue: -1,
	}
	for _, c := range n.Children {
		switch c.XMLName.Local {
		case "audioPackFormatIDRef":
			o.PackRefs = append(o.PackRefs, strings.TrimSpace(c.Text))
		case "audioTrackUIDRef":
			o.TrackRefs = append(o.TrackRefs, strings.TrimSpace(c.Text))
		case "dialogue":
			if v, err := strconv.Atoi(strings.TrimSpace(c.Text)); err == nil {
				o.Dialogue = v
			}
		case "gain":
			if v, err := strconv.ParseFloat(strings.TrimSpace(c.Text), 64); err == nil {
				o.Gain = v
			}
		}
	}
	return o
}

func parsePackFormat(n *xmlNode) *PackFormat {
	pf := &PackFormat{
		ID:   n.attr("audioPackFormatID"),
		Name: n.attr("audioPackFormatName"),
		Type: n.attr("typeLabel"),
	}
	for _, c := range n.children("audioChannelFormatIDRef") {
		pf.ChannelRefs = append(pf.ChannelRefs, strings.TrimSpace(c.Text))
	}
	return pf
}

func parseTrackUID(n *xmlNode) *TrackUID {
	tu := &TrackUID{ID: n.attr("UID")}
	if v, err := strconv.Atoi(n.attr("trackIndex")); err == nil {
		tu.TrackIdx = v
	}
	if c := n.child("audioTrackFormatIDRef"); c != nil {
		tu.TrackFmt = strings.TrimSpace(c.Text)
	}
	return tu
}
