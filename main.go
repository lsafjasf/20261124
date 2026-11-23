package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"ac3norm/internal/batch"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "preview":
		err = runPreview(os.Args[2:])
	case "commit":
		err = runCommit(os.Args[2:])
	case "inspect":
		err = runInspect(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ac3norm: %v\n", err)
		if errors.Is(err, batch.ErrAborted) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `ac3norm - bit-accurate AC-3 dialnorm batch editor (Go stdlib only)

Usage:
  ac3norm preview  -manifest jobs.json   Validate and print the change preview (no writes)
  ac3norm commit   -manifest jobs.json   Re-validate and atomically write every job
  ac3norm inspect  -file stream.ac3      Dump frames, field offsets and payload hashes

Manifest:
  {"jobs":[{"path":"a.ac3","dialnorm":27,"boundaries":[48]}]}

Exit codes: 0 ok, 1 usage/IO error, 3 batch aborted (CRC conflict or duplicated
ad/program boundary frame).
`)
}

func parseManifestFlag(args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "-manifest" && i+1 < len(args) {
			return args[i+1], nil
		}
	}
	return "", errors.New("missing -manifest path")
}

func parseFileFlag(args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "-file" && i+1 < len(args) {
			return args[i+1], nil
		}
	}
	return "", errors.New("missing -file path")
}

func runPreview(args []string) error {
	path, err := parseManifestFlag(args)
	if err != nil {
		return err
	}
	manifest, err := batch.LoadManifest(path)
	if err != nil {
		return err
	}
	previews, err := batch.Preview(manifest)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(previews)
}

func runCommit(args []string) error {
	path, err := parseManifestFlag(args)
	if err != nil {
		return err
	}
	manifest, err := batch.LoadManifest(path)
	if err != nil {
		return err
	}
	if _, err := batch.Preview(manifest); err != nil {
		return err
	}
	previews, err := batch.Commit(manifest)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(previews)
}
