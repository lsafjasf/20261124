package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	be := flag.Bool("be", false, "input 16-bit words are big-endian (default little-endian)")
	channels := flag.Int("channels", 2, "subframes per AES3 frame, used for frame numbers")
	dumpDir := flag.String("dump", "", "directory for dumping raw payloads of unknown data types")
	dumpAll := flag.Bool("dump-all", false, "also dump payloads of known data types")
	flag.Parse()

	var in io.Reader = os.Stdin
	name := "stdin"
	if flag.NArg() > 0 && flag.Arg(0) != "-" {
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		in, name = f, flag.Arg(0)
	}
	raw, err := io.ReadAll(in)
	if err != nil {
		fatal(err)
	}
	if len(raw)%2 != 0 {
		fmt.Fprintf(os.Stderr, "warning: odd byte count %d, trailing byte ignored\n", len(raw))
		raw = raw[:len(raw)-1]
	}

	words := make([]uint16, len(raw)/2)
	for i := range words {
		if *be {
			words[i] = binary.BigEndian.Uint16(raw[2*i:])
		} else {
			words[i] = binary.LittleEndian.Uint16(raw[2*i:])
		}
	}

	segs := Parse(words)
	fmt.Printf("%s: %d words, %d segments\n", name, len(words), len(segs))
	for idx, s := range segs {
		fs, fe := s.frameRange(*channels)
		switch s.Kind {
		case SegPCM:
			fmt.Printf("[%d] PCM   words [%d, %d)  frames [%d, %d)  (%d words)\n",
				idx, s.StartWord, s.EndWord, fs, fe, s.EndWord-s.StartWord)
		case SegError:
			fmt.Printf("[%d] ERROR words [%d, %d)  frames [%d, %d)\n", idx, s.StartWord, s.EndWord, fs, fe)
		case SegBurst:
			fmt.Printf("[%d] BURST words [%d, %d)  frames [%d, %d)  type=0x%02X (%s) mode=%d stream=%d len=%d bits (%d words, %d pad bits) stuffing=%d words\n",
				idx, s.StartWord, s.EndWord, fs, fe,
				s.Pc.DataType, s.TypeName, s.Pc.BurstMode, s.Pc.TypeNumber,
				s.LengthBits, s.PayloadWords, s.PadBits, s.StuffWords)
		}
		for _, e := range s.Errors {
			fmt.Printf("     ! %s\n", e)
		}
		if s.Kind == SegBurst && *dumpDir != "" && (!s.KnownType || *dumpAll) && len(s.Payload) > 0 {
			dumpPayload(*dumpDir, idx, s)
		}
	}
}

func dumpPayload(dir string, idx int, s Segment) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("burst_%03d_w%d_type0x%02X.bin", idx, s.StartWord, s.Pc.DataType))
	buf := make([]byte, 2*len(s.Payload))
	for i, w := range s.Payload {
		binary.LittleEndian.PutUint16(buf[2*i:], w)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("     payload (%d words) -> %s\n", len(s.Payload), path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
