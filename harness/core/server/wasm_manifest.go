package server

import wasmrule "github.com/mnemon-dev/mnemon/harness/core/rule/wasm"

type WASMInspection = wasmrule.Inspection

func InspectWASMManifest(path string) (WASMInspection, error) {
	manifest, wasmBytes, err := wasmrule.LoadManifest(path)
	if err != nil {
		return WASMInspection{}, err
	}
	return wasmrule.ValidateManifest(manifest, wasmBytes)
}
