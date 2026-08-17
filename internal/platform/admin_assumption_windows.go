//go:build windows && !win7

package platform

// Modern Windows reports elevation reliably through the process token, so
// IsAdmin performs the real TokenElevation probe and workflows can surface
// ADMIN_REQUIRED before attempting any SRP write instead of failing with a
// misleading WRITE_FAILED_ROLLED_BACK after an access-denied registry call.
const assumeAdmin = false
