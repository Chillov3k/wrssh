//go:build !pscan && !execass

package subsystems

import "testing"

func TestOptionalModulesAbsentWithoutBuildTags(t *testing.T) {
	for _, name := range []string{"pscan", "execass"} {
		if _, ok := Lookup(name); ok {
			t.Fatalf("module %q is registered without its build tag", name)
		}
	}
}
