package cmd

import "os"

// updateCheckSkippedCommands 是不触发更新提示的子命令(避免边升级边提示、避免污染补全输出)。
var updateCheckSkippedCommands = map[string]bool{
	"update":     true,
	"completion": true,
	"__complete": true,
}

// isUpdateCheckSkippedCommand 报告这次调用的参数是否命中需要抑制更新提示的命令。
func isUpdateCheckSkippedCommand(args []string) bool {
	for _, a := range args {
		if updateCheckSkippedCommands[a] {
			return true
		}
	}
	return false
}

// stderrIsTTY 报告 stderr 是否为交互式终端。
func stderrIsTTY() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
