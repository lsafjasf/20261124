// Command adm is the zero-dependency CLI for the ADM editor page.
//
// Every edit subcommand runs one transaction: validate, atomically rewrite
// only the ADM document, re-parse and check for stale values, and roll back
// the whole transaction on any error. The PCM file is never opened for
// writing.
package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"admeditor/adm"
	"admeditor/pcm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = cmdInit(os.Args[2:])
	case "rename":
		err = cmdRename(os.Args[2:])
	case "set-group":
		err = cmdSetGroup(os.Args[2:])
	case "set-parent":
		err = cmdSetParent(os.Args[2:])
	case "set-points":
		err = cmdSetPoints(os.Args[2:])
	case "add-point":
		err = cmdAddPoint(os.Args[2:])
	case "delete-point":
		err = cmdDeletePoint(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `adm - transactional ADM editor (PCM stays byte-identical)

Usage:
  adm init <pcmPath> <admPath> <objID> <pcmRef>
  adm rename <admPath> <objID> <newName>
  adm set-group <admPath> <objID> <groupID|->
  adm set-parent <admPath> <groupID> <parentID|->
  adm set-points <admPath> <objID> <time:x:y:z,...>
  adm add-point <admPath> <objID> <time> <x> <y> <z>
  adm delete-point <admPath> <objID> <time>
  adm verify <admPath>
`)
}

func cmdInit(args []string) error {
	if len(args) != 4 {
		return fmt.Errorf("init expects <pcmPath> <admPath> <objID> <pcmRef>")
	}
	pcmPath, admPath, objID, ref := args[0], args[1], args[2], args[3]

	chunks := []pcm.Chunk{{
		Type: pcm.ChunkPTS,
		Data: pcm.EncodePTS([]pcm.Sample{{Ref: ref, X: 0, Y: 0, Z: 0}}),
	}}
	if err := pcm.WriteFile(pcmPath, chunks); err != nil {
		return err
	}
	pcmFile, err := pcm.OpenPath(pcmPath)
	if err != nil {
		return err
	}
	sum := pcmFile.SHA256()
	doc := &adm.Document{
		SchemaVersion: adm.SchemaVersion,
		PCMPath:       pcmPath,
		PCMSHA256:     hex.EncodeToString(sum[:]),
		Groups:        []adm.Group{{ID: "g-root", Name: "Root group"}},
		Objects: []adm.Object{{
			ID: objID, Name: objID, GroupID: "g-root", PCMRef: ref,
		}},
	}
	return adm.WriteDoc(admPath, doc)
}

func cmdRename(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("rename expects <admPath> <objID> <newName>")
	}
	return withTx(args[0], func(tx *adm.Transaction) error {
		return tx.RenameObject(args[1], args[2])
	})
}

func cmdSetGroup(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("set-group expects <admPath> <objID> <groupID|->")
	}
	groupID := ""
	if args[2] != "-" {
		groupID = args[2]
	}
	return withTx(args[0], func(tx *adm.Transaction) error {
		return tx.SetObjectGroup(args[1], groupID)
	})
}

func cmdSetParent(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("set-parent expects <admPath> <groupID> <parentID|->")
	}
	parentID := ""
	if args[2] != "-" {
		parentID = args[2]
	}
	return withTx(args[0], func(tx *adm.Transaction) error {
		return tx.SetGroupParent(args[1], parentID)
	})
}

func cmdSetPoints(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("set-points expects <admPath> <objID> <time:x:y:z,...>")
	}
	points, err := parsePoints(args[2])
	if err != nil {
		return err
	}
	return withTx(args[0], func(tx *adm.Transaction) error {
		return tx.SetPoints(args[1], points)
	})
}

func cmdAddPoint(args []string) error {
	if len(args) != 6 {
		return fmt.Errorf("add-point expects <admPath> <objID> <time> <x> <y> <z>")
	}
	point, err := parsePoint(args[2:]...)
	if err != nil {
		return err
	}
	return withTx(args[0], func(tx *adm.Transaction) error {
		return tx.AddPoint(args[1], point)
	})
}

func cmdDeletePoint(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("delete-point expects <admPath> <objID> <time>")
	}
	time, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		return fmt.Errorf("bad timestamp: %w", err)
	}
	return withTx(args[0], func(tx *adm.Transaction) error {
		return tx.DeletePoint(args[1], time)
	})
}

func cmdVerify(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("verify expects <admPath>")
	}
	tx, err := adm.Begin(args[0])
	if err != nil {
		return err
	}
	pcmSum, err := pcm.HashFile(tx.PCMPath())
	if err != nil {
		return err
	}
	if hex.EncodeToString(pcmSum[:]) != tx.PCMSHA256() {
		return adm.ErrPCMChanged
	}
	fmt.Println("OK: document valid, PCM byte-identical")
	return nil
}

func withTx(admPath string, edits func(*adm.Transaction) error) (err error) {
	tx, err := adm.Begin(admPath)
	if err != nil {
		return err
	}
	defer func() {
		if tx.Active() {
			_ = tx.Rollback()
		}
	}()
	if err := edits(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	fmt.Printf("committed %d change(s); PCM unchanged\n", tx.Changes())
	return nil
}

func parsePoints(spec string) ([]adm.Point, error) {
	var points []adm.Point
	for _, item := range strings.Split(spec, ",") {
		if item == "" {
			continue
		}
		p, err := parsePoint(strings.Split(item, ":")...)
		if err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
}

func parsePoint(parts ...string) (adm.Point, error) {
	if len(parts) != 4 {
		return adm.Point{}, fmt.Errorf("point must be time:x:y:z, got %v", parts)
	}
	time, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return adm.Point{}, fmt.Errorf("bad timestamp %q: %w", parts[0], err)
	}
	coords := make([]float64, 3)
	for i, raw := range parts[1:] {
		coords[i], err = strconv.ParseFloat(raw, 64)
		if err != nil {
			return adm.Point{}, fmt.Errorf("bad coordinate %q: %w", raw, err)
		}
	}
	return adm.Point{Time: time, X: coords[0], Y: coords[1], Z: coords[2]}, nil
}
