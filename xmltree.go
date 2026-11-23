package fcpxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

type contentKind uint8

const (
	contentText contentKind = iota
	contentElement
	contentComment
	contentDirective
)

type content struct {
	kind  contentKind
	text  string
	child *Element
}

type prologKind uint8

const (
	prologProcInst prologKind = iota
	prologComment
	prologDirective
	prologText
)

// Element is a lossless XML element. Content preserves mixed text, comments,
// directives, and child order.
type Element struct {
	Name     xml.Name
	Attr     []xml.Attr
	Content  []content
	Path     string
	Children []*Element
}

func (e *Element) AttrValue(name string) (string, bool) {
	if e == nil {
		return "", false
	}
	for _, attr := range e.Attr {
		if attr.Name.Local == name {
			return attr.Value, true
		}
	}
	return "", false
}

func (e *Element) AttrDefault(name, fallback string) string {
	if value, ok := e.AttrValue(name); ok {
		return value
	}
	return fallback
}

func (e *Element) Child(name string) *Element {
	if e == nil {
		return nil
	}
	for _, child := range e.Children {
		if child.Name.Local == name {
			return child
		}
	}
	return nil
}

func (e *Element) ChildrenNamed(name string) []*Element {
	if e == nil {
		return nil
	}
	var result []*Element
	for _, child := range e.Children {
		if child.Name.Local == name {
			result = append(result, child)
		}
	}
	return result
}

// Walk visits elements in document order.
func (e *Element) Walk(fn func(*Element) error) error {
	if e == nil {
		return nil
	}
	if err := fn(e); err != nil {
		return err
	}
	for _, child := range e.Children {
		if err := child.Walk(fn); err != nil {
			return err
		}
	}
	return nil
}

type prolog struct {
	kind prologKind
	text string
}

// Document retains the complete XML document and the parsed element tree.
type Document struct {
	Path    string
	Version string
	Root    *Element
	Raw     []byte
	Prolog  []prolog
	Post    []prolog
}

func ParseDocument(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read FCPXML: %w", err)
	}
	return ParseDocumentBytes(data)
}

func ParseDocumentFile(path string) (*Document, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := ParseDocumentBytes(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	doc.Path = path
	return doc, nil
}

func ParseDocumentBytes(data []byte) (*Document, error) {
	decoder := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	decoder.Strict = true
	doc := &Document{Raw: append([]byte(nil), data...)}
	stack := make([]*Element, 0, 32)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode FCPXML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			node := &Element{Name: value.Name, Attr: append([]xml.Attr(nil), value.Attr...)}
			if len(stack) == 0 {
				if doc.Root != nil {
					return nil, fmt.Errorf("multiple root elements")
				}
				doc.Root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
				parent.Content = append(parent.Content, content{kind: contentElement, child: node})
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unexpected end element %s", value.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			text := string(value)
			if len(stack) == 0 {
				if doc.Root == nil {
					doc.Prolog = append(doc.Prolog, prolog{kind: prologText, text: text})
				} else {
					doc.Post = append(doc.Post, prolog{kind: prologText, text: text})
				}
			} else {
				stack[len(stack)-1].Content = append(stack[len(stack)-1].Content, content{kind: contentText, text: text})
			}
		case xml.Comment:
			if len(stack) == 0 {
				if doc.Root == nil {
					doc.Prolog = append(doc.Prolog, prolog{kind: prologComment, text: string(value)})
				} else {
					doc.Post = append(doc.Post, prolog{kind: prologComment, text: string(value)})
				}
			} else {
				stack[len(stack)-1].Content = append(stack[len(stack)-1].Content, content{kind: contentComment, text: string(value)})
			}
		case xml.ProcInst:
			if len(stack) == 0 {
				item := prolog{
					kind: prologProcInst,
					text: value.Target + "\x00" + string(value.Inst),
				}
				if doc.Root == nil {
					doc.Prolog = append(doc.Prolog, item)
				} else {
					doc.Post = append(doc.Post, item)
				}
			}
		case xml.Directive:
			if len(stack) == 0 {
				if doc.Root == nil {
					doc.Prolog = append(doc.Prolog, prolog{kind: prologDirective, text: string(value)})
				} else {
					doc.Post = append(doc.Post, prolog{kind: prologDirective, text: string(value)})
				}
			} else {
				stack[len(stack)-1].Content = append(stack[len(stack)-1].Content, content{kind: contentDirective, text: string(value)})
			}
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unclosed element %s", stack[len(stack)-1].Name.Local)
	}
	if doc.Root == nil {
		return nil, fmt.Errorf("FCPXML has no root element")
	}
	if doc.Root.Name.Local != "fcpxml" {
		return nil, fmt.Errorf("root element is %q, want fcpxml", doc.Root.Name.Local)
	}
	doc.Version = doc.Root.AttrDefault("version", "")
	doc.assignPaths()
	return doc, nil
}

func (d *Document) assignPaths() {
	var walk func(*Element, string, map[string]int)
	walk = func(node *Element, parent string, sibling map[string]int) {
		sibling[node.Name.Local]++
		node.Path = fmt.Sprintf("%s/%s[%d]", parent, node.Name.Local, sibling[node.Name.Local])
		counts := make(map[string]int)
		for _, child := range node.Children {
			walk(child, node.Path, counts)
		}
	}
	walk(d.Root, "", make(map[string]int))
}

// Bytes re-encodes the document without discarding unknown elements. The
// original raw bytes remain available separately for conservative repair.
func (d *Document) Bytes() ([]byte, error) {
	if d == nil || d.Root == nil {
		return nil, fmt.Errorf("nil FCPXML document")
	}
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	if err := encodeProlog(encoder, d.Prolog); err != nil {
		return nil, err
	}
	if err := encodeElement(encoder, d.Root); err != nil {
		return nil, err
	}
	if err := encodeProlog(encoder, d.Post); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, fmt.Errorf("flush FCPXML: %w", err)
	}
	return buffer.Bytes(), nil
}

func encodeProlog(encoder *xml.Encoder, items []prolog) error {
	for _, item := range items {
		switch item.kind {
		case prologProcInst:
			target, instruction, _ := strings.Cut(item.text, "\x00")
			if err := encoder.EncodeToken(xml.ProcInst{Target: target, Inst: []byte(instruction)}); err != nil {
				return fmt.Errorf("encode processing instruction: %w", err)
			}
		case prologComment:
			if err := encoder.EncodeToken(xml.Comment(item.text)); err != nil {
				return fmt.Errorf("encode comment: %w", err)
			}
		case prologDirective:
			if err := encoder.EncodeToken(xml.Directive(item.text)); err != nil {
				return fmt.Errorf("encode directive: %w", err)
			}
		case prologText:
			if err := encoder.EncodeToken(xml.CharData(item.text)); err != nil {
				return fmt.Errorf("encode text: %w", err)
			}
		}
	}
	return nil
}

func encodeElement(encoder *xml.Encoder, node *Element) error {
	if err := encoder.EncodeToken(xml.StartElement{Name: node.Name, Attr: node.Attr}); err != nil {
		return fmt.Errorf("encode <%s>: %w", node.Name.Local, err)
	}
	for _, item := range node.Content {
		switch item.kind {
		case contentText:
			if err := encoder.EncodeToken(xml.CharData(item.text)); err != nil {
				return err
			}
		case contentElement:
			if err := encodeElement(encoder, item.child); err != nil {
				return err
			}
		case contentComment:
			if err := encoder.EncodeToken(xml.Comment(item.text)); err != nil {
				return err
			}
		case contentDirective:
			if err := encoder.EncodeToken(xml.Directive(item.text)); err != nil {
				return err
			}
		}
	}
	if err := encoder.EncodeToken(xml.EndElement{Name: node.Name}); err != nil {
		return fmt.Errorf("encode </%s>: %w", node.Name.Local, err)
	}
	return nil
}

func sortedAttrFingerprint(node *Element, ignore map[string]bool) string {
	attrs := make([]string, 0, len(node.Attr))
	for _, attr := range node.Attr {
		if ignore[attr.Name.Local] {
			continue
		}
		attrs = append(attrs, attr.Name.Local+"="+attr.Value)
	}
	sort.Strings(attrs)
	return strings.Join(attrs, "|")
}
