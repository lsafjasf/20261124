package fcpxml

import (
	"fmt"
	"math/big"
)

type resourceInfo struct {
	Node          *Element
	ID            string
	Type          string
	Name          string
	UID           string
	Key           string
	FrameDuration *big.Rat
}

type sequenceInfo struct {
	Node          *Element
	Path          string
	Key           string
	Name          string
	FormatRef     string
	FrameDuration *big.Rat
	Duration      *big.Rat
}

type documentIndex struct {
	Resources    map[string]*resourceInfo
	ResourceList []*resourceInfo
	DuplicateIDs []string
	Sequences    []*sequenceInfo
}

func buildDocumentIndex(doc *Document) *documentIndex {
	index := &documentIndex{Resources: make(map[string]*resourceInfo)}
	root := doc.Root
	for _, resources := range root.ChildrenNamed("resources") {
		for _, node := range resources.Children {
			id, _ := node.AttrValue("id")
			if id == "" {
				continue
			}
			info := &resourceInfo{
				Node: node,
				ID:   id,
				Type: node.Name.Local,
				Name: cleanName(node.AttrDefault("name", "")),
				UID:  node.AttrDefault("uid", ""),
			}
			if duration, ok := node.AttrValue("frameDuration"); ok {
				if parsed, err := ParseTime(duration); err == nil {
					info.FrameDuration = parsed.Rat()
				}
			}
			if info.Name == "" {
				info.Name = id
			}
			info.Key = resourceKey(info)
			if _, exists := index.Resources[id]; exists {
				index.DuplicateIDs = append(index.DuplicateIDs, id)
				continue
			}
			index.Resources[id] = info
			index.ResourceList = append(index.ResourceList, info)
		}
	}

	var visit func(*Element, []*Element)
	visit = func(node *Element, ancestors []*Element) {
		next := append(append([]*Element(nil), ancestors...), node)
		if node.Name.Local == "sequence" {
			index.Sequences = append(index.Sequences, sequenceFromNode(node, ancestors, index))
		}
		for _, child := range node.Children {
			visit(child, next)
		}
	}
	visit(root, nil)
	keyCounts := make(map[string]int)
	for _, seq := range index.Sequences {
		keyCounts[seq.Key]++
	}
	keySeen := make(map[string]int)
	for _, seq := range index.Sequences {
		if keyCounts[seq.Key] < 2 {
			continue
		}
		keySeen[seq.Key]++
		seq.Key = fmt.Sprintf("%s[%d]", seq.Key, keySeen[seq.Key])
	}
	return index
}

func sequenceFromNode(node *Element, ancestors []*Element, index *documentIndex) *sequenceInfo {
	seq := &sequenceInfo{
		Node:      node,
		Path:      node.Path,
		Name:      cleanName(node.AttrDefault("name", "")),
		FormatRef: node.AttrDefault("format", ""),
	}
	if duration, ok := node.AttrValue("duration"); ok {
		if parsed, err := ParseTime(duration); err == nil {
			seq.Duration = parsed.Rat()
		}
	}
	if format := index.Resources[seq.FormatRef]; format != nil {
		seq.FrameDuration = format.FrameDuration
	}
	if seq.FrameDuration == nil {
		if duration, ok := node.AttrValue("frameDuration"); ok {
			if parsed, err := ParseTime(duration); err == nil {
				seq.FrameDuration = parsed.Rat()
			}
		}
	}
	project, event := "", ""
	for i := len(ancestors) - 1; i >= 0; i-- {
		switch ancestors[i].Name.Local {
		case "project":
			if project == "" {
				project = cleanName(ancestors[i].AttrDefault("name", ""))
			}
		case "event":
			if event == "" {
				event = cleanName(ancestors[i].AttrDefault("name", ""))
			}
		}
	}
	parts := make([]string, 0, 3)
	for _, value := range []string{project, event, seq.Name} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		seq.Key = node.Path
	} else {
		seq.Key = joinKey(parts)
	}
	return seq
}

func resourceKey(info *resourceInfo) string {
	switch info.Type {
	case "asset":
		if source := firstMediaSource(info.Node); source != "" {
			return "asset:" + source
		}
	case "effect":
		if info.UID != "" {
			return "effect:" + info.UID
		}
	}
	if info.Name != "" {
		return info.Type + ":" + info.Name
	}
	return info.Type + ":" + info.ID
}

func firstMediaSource(node *Element) string {
	if source := node.AttrDefault("src", ""); source != "" {
		return source
	}
	var source string
	_ = node.Walk(func(current *Element) error {
		if source != "" {
			return nil
		}
		if current.Name.Local == "media-rep" {
			source = current.AttrDefault("src", "")
		}
		return nil
	})
	return source
}

func joinKey(parts []string) string {
	result := ""
	for _, part := range parts {
		if result != "" {
			result += "::"
		}
		result += part
	}
	return result
}

func (index *documentIndex) formatForSequence(seq *sequenceInfo) (*resourceInfo, error) {
	if seq.FormatRef == "" {
		return nil, fmt.Errorf("sequence has no format reference")
	}
	format := index.Resources[seq.FormatRef]
	if format == nil {
		return nil, fmt.Errorf("sequence format %q does not resolve", seq.FormatRef)
	}
	if format.Type != "format" {
		return nil, fmt.Errorf("sequence format %q resolves to %s", seq.FormatRef, format.Type)
	}
	return format, nil
}
