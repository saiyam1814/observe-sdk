package observe

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tetratelabs/wabin/binary"
	"github.com/tetratelabs/wabin/leb128"
	"github.com/tetratelabs/wabin/wasm"
)

const wasmInstrVersionMajor = 0
const wasmInstrVersionMinor = 0 // TODO: bump this to match compiler when ready

var errorNoCompatibilityVersion = errors.New("No compatibility versions in module")

// make sure that our function was instrumented with a compatible
// version of wasm-instr
func checkVersion(m *wasm.Module) error {
	var minorGlobal *wasm.Export = nil
	var majorGlobal *wasm.Export = nil
	for _, export := range m.ExportSection {
		if export.Type != wasm.ExternTypeGlobal {
			continue
		}

		if export.Name == "wasm_instr_version_minor" {
			minorGlobal = export
		} else if export.Name == "wasm_instr_version_major" {
			majorGlobal = export
		}
	}

	if minorGlobal == nil || majorGlobal == nil {
		return errorNoCompatibilityVersion
	}

	minor, _, err := leb128.DecodeUint32(bytes.NewReader(m.GlobalSection[minorGlobal.Index].Init.Data))
	if err != nil {
		return err
	}
	major, _, err := leb128.DecodeUint32(bytes.NewReader(m.GlobalSection[majorGlobal.Index].Init.Data))
	if err != nil {
		return err
	}

	if major != wasmInstrVersionMajor || minor < wasmInstrVersionMinor {
		return errors.New(fmt.Sprintf("Expected instrumentation version >= %d.%d but got %d.%d", wasmInstrVersionMajor, wasmInstrVersionMinor, major, minor))
	}

	return nil
}

// Parse the names of the functions out of the
// names custom section in the wasm binary.
func parseNames(data []byte) (map[uint32]string, error) {
	features := wasm.CoreFeaturesV2
	m, err := binary.DecodeModule(data, features)
	if err != nil {
		return nil, err
	}

	// Check for version globals
	if err := checkVersion(m); err != nil && err != errorNoCompatibilityVersion {
		return nil, err
	}

	if m.NameSection == nil {
		return nil, errors.New("Name section not found")
	}

	names := make(map[uint32]string, len(m.NameSection.FunctionNames))

	for _, v := range m.NameSection.FunctionNames {
		names[v.Index] = v.Name
	}

	return names, nil
}
