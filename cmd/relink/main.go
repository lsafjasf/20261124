package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/lsafjasf/20261124/relink"
)

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	var roots stringList
	projectPath := flag.String("project", "", "path to the project JSON file")
	flag.Var(&roots, "root", "media search root; repeat for multiple roots")
	missingProxy := flag.String("missing-proxy", "fail",
		"missing proxy policy: fail or keep")
	acceptAll := flag.Bool("yes", false, "accept every candidate without prompting")
	asJSON := flag.Bool("json", false, "print the relink report as JSON")
	flag.Parse()

	if *projectPath == "" || len(roots) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relink -project PROJECT -root DIR [-root DIR...]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	var approver relink.Approver
	if *acceptAll {
		approver = relink.AcceptAll{}
	} else {
		approver = &promptApprover{
			input: bufio.NewScanner(os.Stdin),
			out:   os.Stdout,
		}
	}
	report, err := relink.Apply(context.Background(), *projectPath, relink.Options{
		Roots:        roots,
		Approver:     approver,
		MissingProxy: relink.MissingProxyPolicy(*missingProxy),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "relink failed: %v\n", err)
		os.Exit(1)
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(1)
		}
		return
	}
	printReport(os.Stdout, report)
}

func printReport(out io.Writer, report *relink.Report) {
	fmt.Fprintf(out, "relinked %d item(s): %d batch, %d individual\n",
		len(report.Links), report.BatchAccepted, report.LowAccepted)
	for _, link := range report.Links {
		fmt.Fprintf(out, "  %s %s [%s]\n    %s\n -> %s\n",
			link.ClipID, link.Role, link.Confidence, link.From, link.To)
	}
	fmt.Fprintf(out, "validated %d media item(s): frame rate, duration, audio offset\n",
		len(report.Checks))
	for _, warning := range report.Warnings {
		fmt.Fprintf(out, "WARNING %s: %s\n", warning.Code, warning.Message)
	}
}

type promptApprover struct {
	input *bufio.Scanner
	out   io.Writer
}

func (approver *promptApprover) ApproveBatch(
	_ context.Context,
	proposals []relink.Proposal,
) (bool, error) {
	fmt.Fprintf(approver.out, "High-confidence batch (%d item(s)):\n", len(proposals))
	for _, proposal := range proposals {
		fmt.Fprintf(approver.out, "  %s %s: %s\n",
			proposal.ClipID, proposal.Role, proposal.Selected.Path)
	}
	fmt.Fprint(approver.out, "Accept this batch? [Y/n] ")
	line, ok := approver.readLine()
	if !ok {
		return false, nil
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "" || line == "y" || line == "yes", nil
}

func (approver *promptApprover) ApproveOne(
	_ context.Context,
	proposal relink.Proposal,
) (relink.Approval, error) {
	fmt.Fprintf(approver.out, "\nLow-confidence item: %s %s\n", proposal.ClipID, proposal.Role)
	fmt.Fprintf(approver.out, "  from: %s\n  reason: %s\n", proposal.OldPath, proposal.Reason)
	for i, candidate := range proposal.Alternatives {
		marker := " "
		if candidate.Path == proposal.Selected.Path {
			marker = "*"
		}
		fmt.Fprintf(approver.out, "  %s %d. %s\n", marker, i+1, candidate.Path)
	}
	fmt.Fprint(approver.out, "Choose 1-N, y for the marked candidate, or n to abort: ")
	line, ok := approver.readLine()
	if !ok {
		return relink.Approval{}, nil
	}
	line = strings.TrimSpace(line)
	switch strings.ToLower(line) {
	case "y", "yes":
		return relink.Approval{Accept: true, CandidatePath: proposal.Selected.Path}, nil
	case "", "n", "no":
		return relink.Approval{}, nil
	}
	index, err := strconv.Atoi(line)
	if err != nil || index < 1 || index > len(proposal.Alternatives) {
		fmt.Fprintln(approver.out, "invalid choice; item rejected")
		return relink.Approval{}, nil
	}
	return relink.Approval{
		Accept:        true,
		CandidatePath: proposal.Alternatives[index-1].Path,
	}, nil
}

func (approver *promptApprover) readLine() (string, bool) {
	if !approver.input.Scan() {
		return "", false
	}
	return approver.input.Text(), true
}
