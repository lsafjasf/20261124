// hdr10-remux 把已确认的 HDR10 元数据迁移到新 Matroska 封装,
// 旧字段彻底移除;任何失败原子回滚,成功后重新解析校验音视频载荷。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"hdr10remux/internal/remux"
)

func main() {
	in := flag.String("i", "", "输入 MKV 文件路径")
	out := flag.String("o", "", "输出 MKV 文件路径")
	quiet := flag.Bool("q", false, "静默模式,不输出进度")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "用法: hdr10-remux -i 输入.mkv -o 输出.mkv [-q]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	logf := func(format string, args ...any) {
		if !*quiet {
			fmt.Printf(format+"\n", args...)
		}
	}

	err := remux.Run(remux.Options{
		InputPath:  *in,
		OutputPath: *out,
		Log:        logf,
	})
	if err != nil {
		switch {
		case errors.Is(err, remux.ErrWritePermission):
			fmt.Fprintf(os.Stderr, "失败(写权限不足): %v\n", err)
		case errors.Is(err, remux.ErrSync):
			fmt.Fprintf(os.Stderr, "失败(容器与码流无法同步): %v\n", err)
		case errors.Is(err, remux.ErrNotConfirmed):
			fmt.Fprintf(os.Stderr, "失败(HDR10 元数据未确认): %v\n", err)
		case errors.Is(err, remux.ErrVerify):
			fmt.Fprintf(os.Stderr, "失败(输出校验未通过): %v\n", err)
		default:
			fmt.Fprintf(os.Stderr, "失败: %v\n", err)
		}
		fmt.Fprintln(os.Stderr, "已回滚: 输入未改动,无残留输出。")
		os.Exit(1)
	}
}
