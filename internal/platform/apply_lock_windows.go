//go:build windows

package platform

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

const applyMutexName = `Global\XCLing-SRP-Apply-v1`

func AcquireApplyLock() (func(), error) {
	name, err := windows.UTF16PtrFromString(applyMutexName)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	result, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	if result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("apply operation is already in progress")
	}
	return func() { _ = windows.ReleaseMutex(h); _ = windows.CloseHandle(h) }, nil
}
