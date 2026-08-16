package training_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedStdlib はドメイン層で使ってよい標準ライブラリ。
// ここに無いものを import したらテストが落ちる。意図的な追加なら明示的にここへ足す。
var allowedStdlib = map[string]bool{
	"context": true,
	"errors":  true,
	"fmt":     true,
	"math":    true,
	"sort":    true,
	"strings": true,
	"time":    true,

	// このテスト自身のため
	"testing":       true,
	"go/ast":        true,
	"go/parser":     true,
	"go/token":      true,
	"os":            true,
	"path/filepath": true,
	"sync":          true,
}

func TestDomain_DependsOnNothingOutside(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ディレクトリを読めない: %v", err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(".", e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s をパースできない: %v", path, err)
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)

			if strings.HasPrefix(p, "github.com/dyoshyy/liftplan-server/") {
				// 自リポジトリ内は domain 配下のみ許可
				if !strings.Contains(p, "/internal/domain/") {
					t.Errorf("%s: ドメイン層が外側に依存している: %s", path, p)
				}
				continue
			}
			if strings.Contains(strings.Split(p, "/")[0], ".") {
				t.Errorf("%s: ドメイン層が外部ライブラリに依存している: %s", path, p)
				continue
			}
			if !allowedStdlib[p] {
				t.Errorf("%s: 許可されていない標準ライブラリ: %s（意図的なら allowedStdlib に追加すること）", path, p)
			}
		}
	}
}
