package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Validate actual checked-in reports against the frozen orchestration contract.
// A skipped measurement stays explicitly unverified instead of becoming a pass.
func TestValidationReportContracts(t *testing.T) {
	raw, err := os.ReadFile("report.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("report-contract.json", spec); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("report-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"native-ui", "ssh-runtime"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", name, "report.json"))
			if err != nil {
				t.Fatal(err)
			}
			var report map[string]any
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(report); err != nil {
				t.Fatal(err)
			}
			if report["workstream"] != name {
				t.Fatal("report ownership does not match directory")
			}
			if report["baseline"] != "53e686f6cfdaae9d162395f49d6de6007b2acc8f" {
				t.Fatal("report did not use frozen baseline")
			}
			for _, entry := range report["metrics"].([]any) {
				m := entry.(map[string]any)
				if m["unit"] == "" || m["value"].(float64) < 0 {
					t.Fatal("invalid metric", m)
				}
			}
			if name == "native-ui" {
				explicit := false
				for _, entry := range report["scenarios"].([]any) {
					s := entry.(map[string]any)
					if s["name"] == "os-input-and-gpu" && s["status"] == "unverified" {
						explicit = true
					}
				}
				if !explicit {
					t.Fatal("unmeasured OS-input/GPU scenario lost its explicit limitation")
				}
			}
			// Required machine/command/skip metadata must not silently disappear.
			for _, field := range []string{"environment", "commands", "limitations"} {
				saved := report[field]
				delete(report, field)
				if schema.Validate(report) == nil {
					t.Errorf("missing %s accepted", field)
				}
				report[field] = saved
			}
		})
	}
}
