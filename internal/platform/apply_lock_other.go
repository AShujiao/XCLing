//go:build !windows

package platform

import "sync"

var applyLock sync.Mutex

func AcquireApplyLock() (func(), error) {
	applyLock.Lock()
	return applyLock.Unlock, nil
}
