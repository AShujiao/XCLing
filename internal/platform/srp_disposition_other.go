//go:build !windows

package platform

func InspectSRPRoot() (SRPRootSnapshot, error) { return SRPRootSnapshot{}, nil }
