package videoedit

import (
	"bufio"
	"fmt"
	"io"
)

// Resolver decides how to proceed when parent/child track timings disagree.
// The application pauses on every detected conflict and asks the resolver.
type Resolver interface {
	Resolve(conflict Conflict) (Choice, error)
}

// AbortResolver always aborts the application at the first conflict.
type AbortResolver struct{}

func (AbortResolver) Resolve(conflict Conflict) (Choice, error) {
	return ChoiceAbort, nil
}

// FixedResolver returns the same choice for every conflict. The choice must be
// valid for each conflict presented, otherwise the application aborts.
type FixedResolver struct {
	Choice Choice
}

func (r FixedResolver) Resolve(conflict Conflict) (Choice, error) {
	for _, choice := range conflict.Choices {
		if choice == r.Choice {
			return r.Choice, nil
		}
	}
	return ChoiceAbort, nil
}

// CLIResolver prompts an operator on a terminal for each conflict.
type CLIResolver struct {
	In      io.Reader
	Out     io.Writer
	scanner *bufio.Scanner
}

func (r *CLIResolver) Resolve(conflict Conflict) (Choice, error) {
	if r.In == nil || r.Out == nil {
		return ChoiceAbort, fmt.Errorf("interactive resolver requires input and output streams")
	}
	fmt.Fprintf(r.Out, "\n[冲突] %s\n", conflict.Message)
	fmt.Fprintf(r.Out, "  父轨道=%s 父片段=%s 子轨道=%s 子片段=%v\n",
		conflict.ParentTrackID, conflict.ParentClipID, conflict.ChildTrackID, conflict.ChildClipIDs)
	for i, choice := range conflict.Choices {
		fmt.Fprintf(r.Out, "  %d) %s\n", i+1, explainChoice(choice))
	}
	fmt.Fprint(r.Out, "请选择 [1]: ")
	if r.scanner == nil {
		r.scanner = bufio.NewScanner(r.In)
	}
	if !r.scanner.Scan() {
		return ChoiceAbort, r.scanner.Err()
	}
	answer := r.scanner.Text()
	if answer == "" {
		return conflict.Choices[0], nil
	}
	for _, choice := range conflict.Choices {
		if answer == string(choice) {
			return choice, nil
		}
	}
	var index int
	if _, err := fmt.Sscanf(answer, "%d", &index); err != nil || index < 1 || index > len(conflict.Choices) {
		fmt.Fprintln(r.Out, "无效选择，按中止处理。")
		return ChoiceAbort, nil
	}
	return conflict.Choices[index-1], nil
}

func explainChoice(choice Choice) string {
	switch choice {
	case ChoiceAbort:
		return string(choice) + " - 中止本次应用，工程回滚保持原样"
	case ChoiceCascade:
		return string(choice) + " - 子片段跟随父片段做相同修剪"
	case ChoiceDetach:
		return string(choice) + " - 保留子片段但标记为脱离父子关系"
	case ChoiceDeleteChildren:
		return string(choice) + " - 删除受影响的子片段"
	case ChoiceSkipParent:
		return string(choice) + " - 跳过父片段操作，保留父片段"
	default:
		return string(choice)
	}
}
