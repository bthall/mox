package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestValidateListsLayoutsSorted pins that mox validate prints layouts in
// name order, not Go's randomized map order.
func TestValidateListsLayoutsSorted(t *testing.T) {
	path := writeTempConfig(t, `layouts:
    zeta:
        panes: [{split: root}]
    alpha:
        panes: [{split: root}]
    mid:
        panes: [{split: root}]
sessions:
    dev:
        hosts: [a]
`)
	for i := 0; i < 5; i++ { // map order varies run to run; one pass could pass by luck
		var out bytes.Buffer
		cmd := NewRootCommand()
		cmd.SetArgs([]string{"validate", "-c", path})
		cmd.SetOut(&out)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("validate: %v", err)
		}
		got := out.String()
		a, m, z := strings.Index(got, "- alpha"), strings.Index(got, "- mid"), strings.Index(got, "- zeta")
		if a < 0 || m < 0 || z < 0 || a > m || m > z {
			t.Fatalf("layouts not sorted:\n%s", got)
		}
	}
}
