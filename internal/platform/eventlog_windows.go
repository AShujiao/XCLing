//go:build windows

package platform

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// eventLogQueryTimeout 单次查询超时上限（防止个别通道卡住）。
const eventLogQueryTimeout = 8 * time.Second

// eventLogMaxOutputBytes 单次查询读取的最大字节数（防止异常大输出）。
const eventLogMaxOutputBytes = 8 << 20 // 8 MiB

// EventLogAvailable 探测事件日志只读查询能力：仅检查 wevtutil 是否可用。只读，无副作用。
func EventLogAvailable() EventLogCapability {
	cap := EventLogCapability{Tool: EventLogTool}
	if _, err := exec.LookPath(EventLogTool); err != nil {
		cap.Available = false
		cap.Reason = "未找到 wevtutil（事件日志查询工具）：" + err.Error()
		return cap
	}
	cap.Available = true
	return cap
}

// QueryEventLog 以**只读**方式查询事件日志，返回 wevtutil 的合并 XML 输出（UTF-8编码）。
// 中文Windows下wevtutil默认输出GBK编码，自动转换为UTF-8。
// 全程只运行 `wevtutil qe`（query-events）；绝不修改/清空/卸载任何日志。
// 通道不存在或权限不足时返回错误，调用方应容忍并跳过该来源。
func QueryEventLog(q EventQuery) (string, error) {
	args := buildWevtutilArgs(q)
	// 纵深防御：断言子命令确为只读 qe。
	if len(args) == 0 || args[0] != EventLogVerbQuery {
		return "", errors.New("refusing to run non-readonly event log command")
	}

	ctx, cancel := context.WithTimeout(context.Background(), eventLogQueryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, EventLogTool, args...)
	hideCommandWindow(cmd)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.New("wevtutil 查询超时：" + q.Channel)
		}
		msg := errBuf.String()
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New("wevtutil 查询失败（" + q.Channel + "）：" + truncateBytes(msg, 300))
	}
	data := out.Bytes()
	if len(data) > eventLogMaxOutputBytes {
		data = data[:eventLogMaxOutputBytes]
	}
	
	// 中文Windows下wevtutil输出为GBK(CP936)编码，转换为UTF-8
	utf8Data, err := gbkToUtf8(data)
	if err != nil {
		// 如果转换失败，返回原始数据
		return string(data), nil
	}
	return string(utf8Data), nil
}

// gbkToUtf8 将GBK编码字节转换为UTF-8
func gbkToUtf8(gbk []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(gbk), simplifiedchinese.GBK.NewDecoder())
	var buf bytes.Buffer
	_, err := buf.ReadFrom(reader)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hideCommandWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func truncateBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
