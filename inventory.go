package fcpxml

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type inventoryBuilder struct {
	doc        *Document
	index      *documentIndex
	inventory  *Inventory
	trackByID  map[string]int
	itemByID   map[string]*ItemInfo
	effectByID map[string]*EffectInfo
}

// Inventory audits one FCPXML document. It never drops XML constructs; the
// findings identify references or timing that cannot be structurally
// reimported.
func (d *Document) Inventory() (*Inventory, error) {
	if d == nil || d.Root == nil {
		return nil, fmt.Errorf("nil FCPXML document")
	}
	index := buildDocumentIndex(d)
	builder := &inventoryBuilder{
		doc:        d,
		index:      index,
		inventory:  &Inventory{Version: d.Version},
		trackByID:  make(map[string]int),
		itemByID:   make(map[string]*ItemInfo),
		effectByID: make(map[string]*EffectInfo),
	}
	builder.addStructureFindings()
	for _, seq := range index.Sequences {
		builder.inventory.Sequences = append(builder.inventory.Sequences, seq.Key)
		builder.scanSequence(seq)
	}
	builder.collectXMLReferences()
	sortFindings(builder.inventory.Findings)
	return builder.inventory, nil
}

func (b *inventoryBuilder) addStructureFindings() {
	if b.doc.Version == "" {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "VERSION_MISSING",
			Severity: SeverityWarning,
			Category: CategoryXML,
			Message:  "fcpxml element has no version attribute",
			Path:     b.doc.Root.Path,
		})
	}
	for _, id := range b.index.DuplicateIDs {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "DUPLICATE_RESOURCE_ID",
			Severity: SeverityLoss,
			Category: CategoryMedia,
			Message:  fmt.Sprintf("resource id %q is declared more than once", id),
			MediaRef: id,
		})
	}
	for _, resource := range b.index.ResourceList {
		if resource.Type == "format" && resource.FrameDuration == nil {
			b.inventory.Findings = append(b.inventory.Findings, Finding{
				Code:     "FORMAT_FRAME_DURATION_MISSING",
				Severity: SeverityWarning,
				Category: CategoryFrame,
				Message:  fmt.Sprintf("format %q has no parseable frameDuration", resource.ID),
				MediaRef: resource.ID,
				Path:     resource.Node.Path,
			})
		}
		if (resource.Type == "asset" || resource.Type == "media") && firstMediaSource(resource.Node) == "" {
			b.inventory.Findings = append(b.inventory.Findings, Finding{
				Code:     "ASSET_MEDIA_SOURCE_MISSING",
				Severity: SeverityWarning,
				Category: CategoryMedia,
				Message:  fmt.Sprintf("%s %q has no media-rep src", resource.Type, resource.ID),
				MediaRef: resource.ID,
				Path:     resource.Node.Path,
			})
		}
	}
}

func (b *inventoryBuilder) scanSequence(seq *sequenceInfo) {
	if _, err := b.index.formatForSequence(seq); err != nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "SEQUENCE_FORMAT_UNRESOLVED",
			Severity: SeverityLoss,
			Category: CategoryMedia,
			Message:  err.Error(),
			MediaRef: seq.FormatRef,
			Path:     seq.Path,
		})
	}
	if seq.FrameDuration == nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "SEQUENCE_FRAME_DURATION_UNKNOWN",
			Severity: SeverityLoss,
			Category: CategoryFrame,
			Message:  "sequence frame duration cannot be resolved; frame positions cannot be audited",
			Path:     seq.Path,
		})
	}
	if seq.Duration == nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "SEQUENCE_DURATION_MISSING",
			Severity: SeverityWarning,
			Category: CategoryFrame,
			Message:  "sequence has no parseable duration",
			Path:     seq.Path,
		})
	}
	for _, child := range seq.Node.Children {
		if child.Name.Local == "spine" {
			b.scanSpine(seq, child, seq.Key+"/spine=primary", "primary")
		}
	}
}

func (b *inventoryBuilder) scanSpine(seq *sequenceInfo, spine *Element, scopeKey, scopeKind string) {
	for _, node := range spine.Children {
		if node.Name.Local == "spine" {
			b.scanSpine(seq, node, scopeKey+"/spine="+node.Path, "nested")
			continue
		}
		if !isTimelineElement(node.Name.Local) {
			continue
		}
		lane := 0
		if raw, ok := node.AttrValue("lane"); ok {
			parsed, err := strconv.Atoi(strings.TrimSpace(raw))
			if err != nil {
				b.inventory.Findings = append(b.inventory.Findings, Finding{
					Code:     "TRACK_LANE_INVALID",
					Severity: SeverityLoss,
					Category: CategoryTrack,
					Message:  fmt.Sprintf("lane %q is not an integer", raw),
					Path:     node.Path,
				})
			} else {
				lane = parsed
			}
		}
		trackID := fmt.Sprintf("%s/lane=%d", scopeKey, lane)
		trackKind := "connected"
		if lane == 0 && scopeKind == "primary" {
			trackKind = "primary"
		} else if scopeKind == "nested" {
			trackKind = "nested"
		}
		b.ensureTrack(trackID, seq, spine.Path, lane, trackKind)
		item := b.addItem(seq, node, trackID)
		if item == nil {
			continue
		}
		b.collectEffects(seq, item, node)
		for _, nested := range node.ChildrenNamed("spine") {
			b.scanSpine(seq, nested, trackID+"/item="+item.ID+"/spine", "nested")
		}
	}
}

func (b *inventoryBuilder) ensureTrack(id string, seq *sequenceInfo, path string, lane int, kind string) {
	if index, ok := b.trackByID[id]; ok {
		b.inventory.Tracks[index].ItemCount++
		return
	}
	b.trackByID[id] = len(b.inventory.Tracks)
	b.inventory.Tracks = append(b.inventory.Tracks, TrackInfo{
		ID:               id,
		Key:              id,
		Path:             path,
		Sequence:         seq.Key,
		Lane:             lane,
		Kind:             kind,
		FrameDuration:    RatString(seq.FrameDuration),
		SequenceDuration: RatString(seq.Duration),
		ItemCount:        1,
	})
}

func (b *inventoryBuilder) addItem(seq *sequenceInfo, node *Element, trackID string) *ItemInfo {
	offset, offsetOK := b.parseAttrTime(node, "offset", seq.FrameDuration)
	if !offsetOK {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "ITEM_OFFSET_MISSING",
			Severity: SeverityLoss,
			Category: CategoryFrame,
			Message:  fmt.Sprintf("%s %q has no valid offset", node.Name.Local, node.AttrDefault("name", "")),
			TrackID:  trackID,
			Path:     node.Path,
		})
	}
	duration, _ := b.parseAttrTime(node, "duration", seq.FrameDuration)
	sourceIn, _ := b.parseAttrTime(node, "start", seq.FrameDuration)
	item := &ItemInfo{
		ID:       node.Path,
		Path:     node.Path,
		TrackID:  trackID,
		Kind:     node.Name.Local,
		Name:     cleanName(node.AttrDefault("name", "")),
		Ref:      node.AttrDefault("ref", ""),
		Offset:   offset,
		Duration: duration,
		SourceIn: sourceIn,
		Node:     node,
	}
	item.MediaKey = b.itemMediaKey(item)
	item.OwnerKey = itemOwnerKey(item)
	if duration == nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "ITEM_DURATION_MISSING",
			Severity: SeverityLoss,
			Category: CategoryFrame,
			Message:  fmt.Sprintf("%s %q has no valid duration", node.Name.Local, item.Name),
			TrackID:  trackID,
			ItemID:   item.ID,
			Path:     node.Path,
			Position: offset,
		})
	}
	b.inventory.Items = append(b.inventory.Items, item)
	b.itemByID[item.ID] = item
	return item
}

func (b *inventoryBuilder) parseAttrTime(node *Element, attr string, frameDuration *big.Rat) (*FramePosition, bool) {
	raw, ok := node.AttrValue(attr)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	parsed, err := ParseTime(raw)
	if err != nil {
		b.inventory.Findings = append(b.inventory.Findings, Finding{
			Code:     "TIME_INVALID",
			Severity: SeverityLoss,
			Category: CategoryFrame,
			Message:  fmt.Sprintf("%s=%q: %v", attr, raw, err),
			Path:     node.Path,
		})
		return nil, false
	}
	return framePosition(parsed.Rat(), frameDuration), true
}

func (b *inventoryBuilder) itemMediaKey(item *ItemInfo) string {
	if item.Ref != "" {
		if resource := b.index.Resources[item.Ref]; resource != nil {
			return resource.Key
		}
		return "unresolved:" + item.Ref
	}
	if source, ok := item.Node.AttrValue("src"); ok && source != "" {
		return "src:" + source
	}
	if item.Name != "" {
		return item.Kind + ":" + item.Name
	}
	return item.Kind
}

func itemOwnerKey(item *ItemInfo) string {
	key := item.Kind + "|" + item.MediaKey
	if item.SourceIn != nil {
		if item.SourceIn.seconds != nil {
			key += "|in=" + RatString(item.SourceIn.seconds)
		}
	}
	if item.Name != "" && item.Ref == "" {
		key += "|name=" + item.Name
	}
	return key
}

func isTimelineElement(name string) bool {
	switch name {
	case "clip", "asset-clip", "ref-clip", "sync-clip", "mc-clip",
		"title", "generator", "gap", "transition", "audition":
		return true
	default:
		return false
	}
}
