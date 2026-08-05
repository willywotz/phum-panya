package model_test

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestModelHasNoGormImport guards the hexagonal dependency rule: the domain
// entity package must not import any infrastructure.
func TestModelHasNoGormImport(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "model.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse model.go: %v", err)
	}
	for _, imp := range f.Imports {
		if strings.Contains(imp.Path.Value, "gorm") {
			t.Fatalf("model.go must not import gorm, found %s", imp.Path.Value)
		}
	}
}
