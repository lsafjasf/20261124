package fcpxml

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (b *inventoryBuilder) collectXMLReferences() {
	_ = b.doc.Root.Walk(func(node *Element) error {
		if ref, ok := node.AttrValue("ref"); ok && strings.TrimSpace(ref) != "" {
			b.recordRef(node, "ref", ref)
		}
		if format, ok := node.AttrValue("format"); ok && strings.TrimSpace(format) != "" {
			b.recordRef(node, "format", format)
		}
		if source, ok := node.AttrValue("src"); ok && strings.TrimSpace(source) != "" {
			b.recordSource(node, source)
		}
		return nil
	})
}

func (b *inventoryBuilder) recordRef(node *Element, attr, value string) {
	resource := b.index.Resources[value]
	item, trackID := b.ownerForPath(node.Path)
	ref := MediaReferenceInfo{
		ID:       node.Path + "#" + attr,
		Path:     node.Path,
		Kind:     referenceKind(node, attr),
		Value:    value,
		Resolved: resource != nil,
		TrackID:  trackID,
	}
	if item != nil {
		ref.ItemID = item.ID
		ref.OwnerKey = item.OwnerKey
		ref.Position = item.Offset
	}
	if resource != nil {
		ref.ResourceType = resource.Type
		ref.ResourceName = resource.Name
		ref.ResourceKey = resource.Key
	}
	b.inventory.MediaRefs = append(b.inventory.MediaRefs, ref)
	if resource != nil && attr == "ref" && isMediaItemElement(node.Name.Local) &&
		(resource.Type == "format" || resource.Type == "effect") {
		finding := Finding{
			Code:     "REF_TYPE_MISMATCH",
			Severity: SeverityLoss,
			Category: CategoryMedia,
			Message:  fmt.Sprintf("item ref %q resolves to %s", value, resource.Type),
			MediaRef: value,
			Path:     node.Path,
			TrackID:  trackID,
		}
		if item != nil {
			finding.ItemID = item.ID
			finding.Position = item.Offset
		}
		b.inventory.Findings = append(b.inventory.Findings, finding)
	}
	if resource != nil || attr == "format" || isEffectRefElement(node.Name.Local) {
		return
	}
	finding := Finding{
		Code:     "REF_UNRESOLVED",
		Severity: SeverityLoss,
		Category: CategoryMedia,
		Message:  fmt.Sprintf("%s %q does not resolve to a resource", attr, value),
		MediaRef: value,
		Path:     node.Path,
		TrackID:  trackID,
	}
	if item != nil {
		finding.ItemID = item.ID
		finding.Position = item.Offset
	}
	b.inventory.Findings = append(b.inventory.Findings, finding)
}

func isMediaItemElement(name string) bool {
	switch name {
	case "clip", "asset-clip", "ref-clip", "sync-clip", "mc-clip", "transition":
		return true
	default:
		return false
	}
}

func (b *inventoryBuilder) recordSource(node *Element, source string) {
	item, trackID := b.ownerForPath(node.Path)
	ref := MediaReferenceInfo{
		ID:       node.Path + "#src",
		Path:     node.Path,
		Kind:     node.Name.Local + "-src",
		Value:    source,
		Resolved: true,
		URI:      source,
		TrackID:  trackID,
	}
	if item != nil {
		ref.ItemID = item.ID
		ref.OwnerKey = item.OwnerKey
		ref.Position = item.Offset
	}
	ref.FileState, ref.Reason = b.fileState(source)
	b.inventory.MediaRefs = append(b.inventory.MediaRefs, ref)
	if ref.FileState == "missing" {
		finding := Finding{
			Code:     "MEDIA_FILE_MISSING",
			Severity: SeverityWarning,
			Category: CategoryMedia,
			Message:  fmt.Sprintf("media file does not exist: %s", source),
			MediaRef: source,
			Path:     node.Path,
			TrackID:  trackID,
		}
		if item != nil {
			finding.ItemID = item.ID
			finding.Position = item.Offset
		}
		b.inventory.Findings = append(b.inventory.Findings, finding)
	}
}

func (b *inventoryBuilder) ownerForPath(path string) (*ItemInfo, string) {
	var owner *ItemInfo
	for _, item := range b.inventory.Items {
		if path == item.Path || strings.HasPrefix(path, item.Path+"/") {
			if owner == nil || len(item.Path) > len(owner.Path) {
				owner = item
			}
		}
	}
	if owner == nil {
		return nil, ""
	}
	return owner, owner.TrackID
}

func referenceKind(node *Element, attr string) string {
	if attr == "format" {
		return "sequence-format"
	}
	switch node.Name.Local {
	case "filter-video", "filter-audio", "filter", "effect":
		return "effect-ref"
	case "clip", "asset-clip", "ref-clip", "sync-clip", "mc-clip", "title", "generator", "transition":
		return "item-ref"
	case "video", "audio":
		return "media-component-ref"
	default:
		return node.Name.Local + "-ref"
	}
}

func isEffectRefElement(name string) bool {
	switch name {
	case "filter-video", "filter-audio", "filter", "effect":
		return true
	default:
		return false
	}
}

func (b *inventoryBuilder) fileState(source string) (string, string) {
	if strings.TrimSpace(source) == "" {
		return "invalid", "empty source"
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return "invalid", err.Error()
	}
	if parsed.Scheme != "" && parsed.Scheme != "file" {
		return "remote", "non-file URL is not checked locally"
	}
	path := parsed.Path
	if parsed.Scheme == "" {
		path = source
	}
	if parsed.Scheme == "file" && parsed.Host != "" && parsed.Host != "localhost" {
		path = "//" + parsed.Host + parsed.Path
	}
	if unescaped, err := url.PathUnescape(path); err == nil {
		path = unescaped
	}
	if path == "" {
		return "invalid", "file URL has no path"
	}
	path = filepath.FromSlash(path)
	if !filepath.IsAbs(path) && b.doc.Path != "" {
		path = filepath.Join(filepath.Dir(b.doc.Path), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", path
		}
		return "error", err.Error()
	}
	if info.IsDir() {
		return "directory", path
	}
	return "present", path
}
