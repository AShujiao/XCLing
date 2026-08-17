//go:build windows

package platform

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const netSetupDomainName = 3

func IsDomainJoined() (bool, error) {
	var name *uint16
	var status uint32
	if err := windows.NetGetJoinInformation(nil, &name, &status); err != nil {
		return false, err
	}
	if name != nil {
		_ = windows.NetApiBufferFree((*byte)(unsafe.Pointer(name)))
	}
	return status == netSetupDomainName, nil
}
