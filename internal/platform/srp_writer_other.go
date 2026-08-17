//go:build !windows

package platform

import (
	"errors"

	pgapply "XCLing/internal/apply"
	"XCLing/internal/model"
)

func SRPKeyExists() (bool, error)     { return false, nil }
func WriteSRPPlan(pgapply.Plan) error { return errors.New("SRP 仅支持 Windows") }
func WriteSRPPlanFrom(pgapply.Plan, SRPRootSnapshot) error {
	return errors.New("SRP 仅支持 Windows")
}
func ReadSRPPlan() (pgapply.Plan, error) { return pgapply.Plan{}, errors.New("SRP 仅支持 Windows") }
func RemoveSRPPlan(pgapply.Plan) error   { return errors.New("SRP 仅支持 Windows") }
func RestoreSRPBeforeState(pgapply.Plan, string) error {
	return errors.New("SRP 仅支持 Windows")
}
func WriteSRPPlanReplacing(pgapply.Plan, model.RegistryTreeSnapshot) error {
	return errors.New("SRP 仅支持 Windows")
}
func SetSRPProtectionState(pgapply.Plan, bool) error { return errors.New("SRP 仅支持 Windows") }
func SnapshotSRPRegistry() (model.RegistryTreeSnapshot, error) {
	return model.RegistryTreeSnapshot{}, errors.New("SRP 仅支持 Windows")
}
func RestoreSRPRegistrySnapshot(model.RegistryTreeSnapshot) error {
	return errors.New("SRP 仅支持 Windows")
}
