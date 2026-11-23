// pgsgen writes the binary PGS test fixtures into pgs/testdata.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"pgsrebuild/internal/fixture"
)

func main() {
	dir := filepath.Join("pgs", "testdata")
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	for name, data := range fixture.Streams() {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	}
}
