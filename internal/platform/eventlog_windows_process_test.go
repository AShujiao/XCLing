//go:build windows

package platform

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHideCommandWindowUsesNoWindowProcessFlags(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	hideCommandWindow(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatal("event query child process must hide its window")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("event query child process must use CREATE_NO_WINDOW")
	}
}
