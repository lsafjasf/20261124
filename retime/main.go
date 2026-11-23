package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "apply":
		err = cmdApply(os.Args[2:])
	case "undo":
		err = cmdUndo(os.Args[2:])
	case "confirm":
		err = cmdConfirm(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `retime - map old sequence positions to a retimed sequence (stdlib only)

Usage:
  retime apply   <project.json> [manifest.json]   remap events; print/write manifest with undo snapshot
  retime undo    <manifest.json> [project.json]   restore original speed curve and all events
  retime confirm <manifest.json> <choices.json> [out.json]
                                                  resolve pending events with manual choices
`)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func cmdApply(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("apply needs a project file")
	}
	var p Project
	if err := readJSON(args[0], &p); err != nil {
		return err
	}
	m, err := ApplyProject(p)
	if err != nil {
		return err
	}
	out := ""
	if len(args) > 1 {
		out = args[1]
	}
	if err := writeJSON(out, m); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "apply: %d unique, %d pending manual confirmation, new duration %d frames\n",
		len(m.UniqueMapped), len(m.Pending), m.NewDuration)
	return nil
}

func cmdUndo(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("undo needs a manifest file")
	}
	var m Manifest
	if err := readJSON(args[0], &m); err != nil {
		return err
	}
	out := ""
	if len(args) > 1 {
		out = args[1]
	}
	return writeJSON(out, Undo(m))
}

func cmdConfirm(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("confirm needs a manifest file and a choices file")
	}
	var m Manifest
	if err := readJSON(args[0], &m); err != nil {
		return err
	}
	var choices []Confirmation
	if err := readJSON(args[1], &choices); err != nil {
		return err
	}
	updated, err := Confirm(m, choices)
	if err != nil {
		return err
	}
	out := ""
	if len(args) > 2 {
		out = args[2]
	}
	if err := writeJSON(out, updated); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "confirm: %d still pending\n", len(updated.Pending))
	return nil
}
