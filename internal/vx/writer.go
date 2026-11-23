package vx

import (
	"encoding/binary"
	"encoding/json"
	"io"
)

// Writer serializes a VX1 stream.
type Writer struct {
	w      io.Writer
	header Header
}

// NewWriter writes magic and the JSON header.
func NewWriter(w io.Writer, header Header) (*Writer, error) {
	if _, err := w.Write(magic); err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte{1}); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(header)
	if err != nil {
		return nil, err
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(raw)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return nil, err
	}
	if _, err := w.Write(raw); err != nil {
		return nil, err
	}
	return &Writer{w: w, header: header}, nil
}

// WriteFrame appends one video block.
func (w *Writer) WriteFrame(f Frame) error {
	plane := w.header.Width * w.header.Height
	if len(f.Y) != plane || len(f.U) != plane || len(f.V) != plane {
		return io.ErrShortBuffer
	}
	payload := make([]byte, 0, frameHeaderSize+3*plane)
	var head [frameHeaderSize]byte
	binary.LittleEndian.PutUint32(head[0:4], f.Num)
	binary.LittleEndian.PutUint64(head[4:12], uint64(f.PTS))
	var flags byte
	if f.Key {
		flags |= 1
	}
	if f.Cut {
		flags |= 2
	}
	head[12] = flags
	payload = append(payload, head[:]...)
	payload = append(payload, f.Y...)
	payload = append(payload, f.U...)
	payload = append(payload, f.V...)
	return w.writeBlock(blockVideo, payload)
}

// WriteAudio appends one audio block.
func (w *Writer) WriteAudio(p AudioPacket) error {
	payload := make([]byte, 0, 10+2*len(p.Samples))
	var head [10]byte
	binary.LittleEndian.PutUint64(head[0:8], p.SampleStart)
	binary.LittleEndian.PutUint16(head[8:10], uint16(len(p.Samples)))
	payload = append(payload, head[:]...)
	var buf [2]byte
	for _, s := range p.Samples {
		binary.LittleEndian.PutUint16(buf[:], uint16(s))
		payload = append(payload, buf[:]...)
	}
	return w.writeBlock(blockAudio, payload)
}

// WriteSubtitle appends one subtitle block.
func (w *Writer) WriteSubtitle(s Subtitle) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return w.writeBlock(blockSubtitle, raw)
}

// Close writes the terminator.
func (w *Writer) Close() error {
	_, err := w.w.Write([]byte{blockEnd, 0, 0, 0, 0})
	return err
}

func (w *Writer) writeBlock(tag byte, payload []byte) error {
	if _, err := w.w.Write([]byte{tag}); err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := w.w.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := w.w.Write(payload)
	return err
}
