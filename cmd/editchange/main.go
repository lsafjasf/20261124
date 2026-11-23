package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"editchange/videoedit"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("missing subcommand")
	}
	switch args[0] {
	case "apply":
		return applyCmd(args[1:])
	case "undo":
		return undoCmd(args[1:])
	case "demo":
		return demoCmd()
	case "version":
		fmt.Println("editchange 0.1.0")
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `editchange - 可恢复的剪辑变更单应用器

用法:
  editchange apply -project p.json -order o.json [-journal j.json] [-mode abort|interactive|fixed:<choice>]
  editchange undo  -project p.json -journal j.json
  editchange demo`)
}

func applyCmd(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	projectPath := fs.String("project", "", "project JSON file")
	orderPath := fs.String("order", "", "change order JSON file")
	journalPath := fs.String("journal", "", "optional path to persist the undo log")
	mode := fs.String("mode", "abort", "abort | interactive | fixed:<choice>")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *projectPath == "" || *orderPath == "" {
		return fmt.Errorf("apply requires -project and -order")
	}

	project, err := loadProject(*projectPath)
	if err != nil {
		return err
	}
	order, err := loadOrder(*orderPath)
	if err != nil {
		return err
	}
	resolver, err := buildResolver(*mode)
	if err != nil {
		return err
	}

	result, err := videoedit.Apply(project, order, videoedit.ApplyOptions{Resolver: resolver})
	if err != nil {
		return err
	}
	if err := saveJSON(*projectPath, project); err != nil {
		return err
	}
	if *journalPath != "" {
		data, err := result.Journal.Marshal()
		if err != nil {
			return err
		}
		if err := os.WriteFile(*journalPath, data, 0o644); err != nil {
			return fmt.Errorf("write journal: %w", err)
		}
	}
	fmt.Printf("应用完成: 写入 %d 项, 跳过 %d 项, 冲突 %d 次, 撤销日志 %d 条\n",
		len(result.Applied), len(result.Skipped), len(result.Conflicts), len(result.Journal.Entries))
	for _, skip := range result.Skipped {
		fmt.Printf("  跳过 %s: %s\n", skip.ClipID, skip.Reason)
	}
	return nil
}

func undoCmd(args []string) error {
	fs := flag.NewFlagSet("undo", flag.ContinueOnError)
	projectPath := fs.String("project", "", "project JSON file")
	journalPath := fs.String("journal", "", "undo log JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *projectPath == "" || *journalPath == "" {
		return fmt.Errorf("undo requires -project and -journal")
	}
	project, err := loadProject(*projectPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*journalPath)
	if err != nil {
		return fmt.Errorf("read journal: %w", err)
	}
	journal, err := videoedit.ParseJournal(data)
	if err != nil {
		return err
	}
	if err := journal.Rollback(project); err != nil {
		return err
	}
	if err := saveJSON(*projectPath, project); err != nil {
		return err
	}
	fmt.Printf("已按撤销日志恢复 %d 条写入\n", len(journal.Entries))
	return nil
}

func buildResolver(mode string) (videoedit.Resolver, error) {
	switch {
	case mode == "abort":
		return videoedit.AbortResolver{}, nil
	case mode == "interactive":
		return &videoedit.CLIResolver{In: os.Stdin, Out: os.Stdout}, nil
	case len(mode) > 6 && mode[:6] == "fixed:":
		return videoedit.FixedResolver{Choice: videoedit.Choice(mode[6:])}, nil
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
}

func loadProject(path string) (*videoedit.Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project: %w", err)
	}
	var project videoedit.Project
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("parse project: %w", err)
	}
	return &project, nil
}

func loadOrder(path string) (*videoedit.ChangeOrder, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read order: %w", err)
	}
	return videoedit.ParseOrder(data)
}

func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
