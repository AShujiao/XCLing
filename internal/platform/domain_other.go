//go:build !windows

package platform

func IsDomainJoined() (bool, error) { return false, nil }
