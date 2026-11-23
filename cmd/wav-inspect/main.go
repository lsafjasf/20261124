package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"wav"
)

type report struct {
	File              string      `json:"file"`
	Valid             bool        `json:"valid"`
	FirstSampleOffset *int64      `json:"first_sample_offset,omitempty"`
	Issues            []wav.Issue `json:"issues"`
	Header            *wav.Header `json:"header,omitempty"`
	Error             string      `json:"error,omitempty"`
}

func main() {
	compact := flag.Bool("compact", false, "write compact JSON instead of indented JSON")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-compact] FILE.wav\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	path := flag.Arg(0)
	header, err := wav.ParseFile(path)
	result := report{
		File:   path,
		Issues: []wav.Issue{},
		Header: header,
	}
	if header != nil {
		result.Valid = header.Valid
		result.Issues = header.Issues
		if validationErr, ok := err.(*wav.ValidationError); ok {
			if first := validationErr.First(); first != nil && first.FirstSampleOffset >= 0 {
				offset := first.FirstSampleOffset
				result.FirstSampleOffset = &offset
			}
		}
	}
	if err != nil && header == nil {
		result.Error = err.Error()
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if !*compact {
		encoder.SetIndent("", "  ")
	}
	if encodeErr := encoder.Encode(result); encodeErr != nil {
		fmt.Fprintf(os.Stderr, "encode result: %v\n", encodeErr)
		os.Exit(2)
	}
	if !result.Valid {
		os.Exit(1)
	}
}
