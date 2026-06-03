package kernel

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

type SchemaGuard struct {
	Required map[contract.ResourceKind][]string
}

func DefaultSchemaGuard() SchemaGuard {
	return SchemaGuard{Required: map[contract.ResourceKind][]string{"memory": {"content"}, "goal": {"statement"}, "skill": {"name"}}}
}
func (g SchemaGuard) Validate(kind contract.ResourceKind, fields map[string]any) error {
	for _, f := range g.Required[kind] {
		if _, ok := fields[f]; !ok {
			return fmt.Errorf("%w: %s requires %q", errSchema, kind, f)
		}
	}
	return nil
}
