package mm

import "phenix/types"

func ReadScriptFromFile(filename string) error {
	return DefaultMM.ReadScriptFromFile(filename)
}

func ClearNamespace(ns string) error {
	return DefaultMM.ClearNamespace(ns)
}

func GetVMInfo(opts ...Option) types.VMs {
	return DefaultMM.GetVMInfo(opts...)
}

func StartVM(opts ...Option) error {
	return DefaultMM.StartVM(opts...)
}

func StopVM(opts ...Option) error {
	return DefaultMM.StopVM(opts...)
}

func KillVM(opts ...Option) error {
	return DefaultMM.KillVM(opts...)
}

func RedeployVM(opts ...Option) error {
	return DefaultMM.RedeployVM(opts...)
}

func StartVMCapture(opts ...Option) error {
	return DefaultMM.StartVMCapture(opts...)
}

func StopVMCapture(opts ...Option) error {
	return DefaultMM.StopVMCapture(opts...)
}

func GetExperimentCaptures(opts ...Option) []types.Capture {
	return DefaultMM.GetExperimentCaptures(opts...)
}

func GetVMCaptures(opts ...Option) []types.Capture {
	return DefaultMM.GetVMCaptures(opts...)
}
