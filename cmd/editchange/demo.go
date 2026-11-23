package main

import (
	"encoding/json"
	"fmt"
	"time"

	"editchange/videoedit"
)

func demoCmd() error {
	orders := [][]videoedit.Operation{
		{
			{ClipID: "c2", Action: videoedit.ActionTrimEnd, Delta: ms(400)},
			{ClipID: "c1", Action: videoedit.ActionTrimEnd, Delta: ms(-300)},
			{ClipID: "c2", Action: videoedit.ActionTrimStart, Delta: ms(200)},
		},
		{
			{ClipID: "c2", Action: videoedit.ActionTrimStart, Delta: ms(200)},
			{ClipID: "c1", Action: videoedit.ActionTrimEnd, Delta: ms(-300)},
			{ClipID: "c2", Action: videoedit.ActionTrimEnd, Delta: ms(400)},
		},
		{
			{ClipID: "c1", Action: videoedit.ActionTrimEnd, Delta: ms(-300)},
			{ClipID: "c2", Action: videoedit.ActionTrimEnd, Delta: ms(400)},
			{ClipID: "c2", Action: videoedit.ActionTrimStart, Delta: ms(200)},
		},
	}

	snapshots := make([]string, 0, len(orders))
	for i, ops := range orders {
		project := demoProject()
		order := &videoedit.ChangeOrder{ID: fmt.Sprintf("demo-%d", i), Operations: ops}
		result, err := videoedit.Apply(project, order, videoedit.ApplyOptions{
			Resolver: videoedit.FixedResolver{Choice: videoedit.ChoiceDetach},
		})
		if err != nil {
			return fmt.Errorf("demo run %d failed: %w", i+1, err)
		}
		data, _ := json.Marshal(project)
		snapshots = append(snapshots, string(data))
		fmt.Printf("顺序 %d: 应用 %d 项, 撤销日志 %d 条, 冲突 %d 次\n",
			i+1, len(result.Applied), len(result.Journal.Entries), len(result.Conflicts))
	}
	if snapshots[0] != snapshots[1] || snapshots[1] != snapshots[2] {
		return fmt.Errorf("three application orders produced different results")
	}
	fmt.Println("三种顺序收敛到同一工程状态 ✓")
	return nil
}

func demoProject() *videoedit.Project {
	return &videoedit.Project{
		ID:   "demo",
		Name: "父子轨道演示工程",
		Tracks: []videoedit.Track{
			{
				ID:   "video",
				Name: "主视频轨",
				Clips: []videoedit.Clip{
					{ID: "c1", Name: "A", Start: ms(0), Length: ms(2000), SourceIn: ms(0)},
					{ID: "c2", Name: "B", Start: ms(2000), Length: ms(3000), SourceIn: ms(0)},
				},
			},
			{
				ID:       "subtitles",
				Name:     "字幕轨",
				ParentID: "video",
				Clips: []videoedit.Clip{
					{ID: "s1", Name: "标题", Start: ms(300), Length: ms(1000), SourceIn: ms(0)},
					{ID: "s2", Name: "解说", Start: ms(2200), Length: ms(2000), SourceIn: ms(0)},
				},
			},
		},
	}
}

func ms(v int64) videoedit.Duration {
	return videoedit.Duration{Duration: time.Duration(v) * time.Millisecond}
}
