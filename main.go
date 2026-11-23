package main

import (
	"fmt"
	"os"
)

func main() {
	cmd := "report"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "report":
		text := BuildPostmortem()
		fmt.Print(text)
		if err := os.WriteFile("POSTMORTEM.md", []byte(text), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write POSTMORTEM.md:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "\nwrote POSTMORTEM.md")
	case "stages":
		buggy := &Pipeline{Buggy: true}
		dumps, _ := buggy.RunTimeline()
		for _, d := range dumps {
			fmt.Print(FormatStageTable(d))
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: hdrsdr [report|stages]")
		os.Exit(2)
	}
}
