package offlinelog

import (
	"bytes"
	"encoding/json"
	"io"
)

// EncodeLog 将一份离线日志序列化为 JSON 数组。
// 空日志编码为 "[]"，与 DecodeLog 对称，绝不产生 nil 歧义。
func EncodeLog(events []Event) ([]byte, error) {
	if events == nil {
		events = []Event{}
	}
	return json.Marshal(events)
}

// DecodeLog 从 JSON 字节流恢复离线日志；空数组返回长度为 0 的切片。
func DecodeLog(data []byte) ([]Event, error) {
	var events []Event
	if len(bytes.TrimSpace(data)) == 0 {
		return []Event{}, nil
	}
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	if events == nil {
		events = []Event{}
	}
	return events, nil
}

// WriteLog 将日志写入流，便于落盘到本地离线缓冲文件。
func WriteLog(w io.Writer, events []Event) error {
	data, err := EncodeLog(events)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadLog 从流中读取一份离线日志。
func ReadLog(r io.Reader) ([]Event, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return DecodeLog(data)
}
