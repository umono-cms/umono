package themeapi

import (
	"testing"

	"github.com/umono-cms/themecore"
)

func TestBuiltinSetInCompono(t *testing.T) {
	if err := themecore.BuiltinSet(); err != nil {
		t.Fatal(err)
	}
}
