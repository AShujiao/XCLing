//go:build windows && win7

package platform

// The Win7 compatibility build does not gate workflows on UAC token probing.
// Registry ACLs remain authoritative when an operation actually writes SRP.
const assumeAdmin = true
