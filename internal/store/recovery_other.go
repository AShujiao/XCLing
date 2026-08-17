//go:build !windows

package store

func secureRecoveryPath(string) error { return nil }
