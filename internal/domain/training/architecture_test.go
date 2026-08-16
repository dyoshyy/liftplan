package training_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const modulePath = "github.com/dyoshyy/liftplan-server"

// productionStdlib はドメイン層の本番コードで使ってよい標準ライブラリ。
//
// ここに無いものを import したらテストが落ちる。意図的な追加なら明示的にここへ足すこと。
// os / path/filepath / net/http / database/sql が入っていないのは意図的で、
// ドメイン層が外界に触れないという制約そのものを表している。
var productionStdlib = map[string]bool{
	"context": true,
	"errors":  true,
	"fmt":     true,
	"math":    true,
	"sort":    true,
	"strings": true,
	"time":    true,
}

// testOnlyStdlib はテストファイルにのみ追加で許可する標準ライブラリ。
//
// 本番コードと分けているのは、この検査テスト自身が必要とする os や go/parser を
// 本番コードにも許してしまうと、ドメイン層がファイルシステムを触れるようになるため。
var testOnlyStdlib = map[string]bool{
	"testing":       true,
	"go/ast":        true,
	"go/parser":     true,
	"go/token":      true,
	"io/fs":         true,
	"path/filepath": true,
	"runtime":       true,
	"strconv":       true,
	"sync":          true,
}

// domainRoot はこのテストファイルの位置から internal/domain を解決する。
//
// os.Getwd に頼らないのは、`go test -c` したバイナリを別ディレクトリで実行すると
// 走査対象が消えてテストが素通りしてしまうため。
func domainRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("テストファイルの位置を解決できない")
	}
	// .../internal/domain/training/architecture_test.go → .../internal/domain
	return filepath.Dir(filepath.Dir(thisFile))
}

func TestDomain_DependsOnNothingOutside(t *testing.T) {
	root := domainRoot(t)
	fset := token.NewFileSet()
	scanned := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		scanned++

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		isTest := strings.HasSuffix(d.Name(), "_test.go")
		for _, imp := range file.Imports {
			checkImport(t, rel, strings.Trim(imp.Path.Value, `"`), isTest)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ドメイン層の走査に失敗: %v", err)
	}

	// 走査対象が0件なら、検査したつもりで何も守っていない状態になる。
	if scanned == 0 {
		t.Fatalf("%s に .go ファイルが1つも見つからない。検査が空振りしている", root)
	}
	t.Logf("%d ファイルを検査した", scanned)
}

// MustDate は不正な入力で panic する。コンパイル時に確定するリテラル専用であり、
// 本番コードに現れてはならない。外部入力は ParseDate か FromTime を通すこと。
//
// 「テスト専用」を規約で守らせると必ず破られるので、AST で機械的に禁止する。
func TestDomain_MustDateIsTestOnly(t *testing.T) {
	root := domainRoot(t)
	fset := token.NewFileSet()
	checked := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// テストコードでの使用は正当。宣言そのものがある date.go も対象外。
		if strings.HasSuffix(d.Name(), "_test.go") || d.Name() == "date.go" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		checked++

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.Ident:
				if v.Name == "MustDate" {
					t.Errorf("%s:%d: 本番コードで MustDate を使っている。外部入力は ParseDate か FromTime を通すこと",
						rel, fset.Position(v.Pos()).Line)
				}
			case *ast.SelectorExpr:
				if v.Sel != nil && v.Sel.Name == "MustDate" {
					t.Errorf("%s:%d: 本番コードで MustDate を使っている。外部入力は ParseDate か FromTime を通すこと",
						rel, fset.Position(v.Pos()).Line)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("ドメイン層の走査に失敗: %v", err)
	}
	t.Logf("%d ファイルを検査した", checked)
}

func checkImport(t *testing.T, file, importPath string, isTest bool) {
	t.Helper()

	if importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/") {
		rest := strings.TrimPrefix(strings.TrimPrefix(importPath, modulePath), "/")
		if rest != "internal/domain" && !strings.HasPrefix(rest, "internal/domain/") {
			t.Errorf("%s: ドメイン層が外側に依存している: %s", file, importPath)
		}
		return
	}

	// 先頭セグメントにドットを含めばホスト名、すなわち外部モジュール。
	if strings.Contains(strings.Split(importPath, "/")[0], ".") {
		t.Errorf("%s: ドメイン層が外部ライブラリに依存している: %s", file, importPath)
		return
	}

	if productionStdlib[importPath] {
		return
	}
	if isTest && testOnlyStdlib[importPath] {
		return
	}

	if isTest {
		t.Errorf("%s: 許可されていない標準ライブラリ: %s（意図的なら testOnlyStdlib に追加すること）",
			file, importPath)
		return
	}
	t.Errorf("%s: 本番コードで許可されていない標準ライブラリ: %s（意図的なら productionStdlib に追加すること）",
		file, importPath)
}

// StimulusProfile は構造体の値コピーでも内部マップを共有する。
// パッケージ内で m に書き込むと、そのエンティティの刺激分布が黙って壊れ、
// 同じポインタを共有する全セッションに波及する。
//
// 消費者（残差計算・補助種目選択・セッション生成）は全て同じパッケージにいるので、
// 「書くな」を規約で守らせても必ず破られる。AST で機械的に禁止する。
func TestDomain_StimulusProfileIsNotMutated(t *testing.T) {
	root := domainRoot(t)
	fset := token.NewFileSet()
	checked := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// 宣言とコンストラクタがある exercise.go だけが m を組み立てられる。
		if d.Name() == "exercise.go" {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		checked++

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				index, ok := lhs.(*ast.IndexExpr)
				if !ok {
					continue
				}
				sel, ok := index.X.(*ast.SelectorExpr)
				if !ok || sel.Sel == nil || sel.Sel.Name != "m" {
					continue
				}
				t.Errorf("%s:%d: 内部マップ m への書き込みは禁止されている。"+
					"エンティティの状態が黙って壊れる",
					rel, fset.Position(assign.Pos()).Line)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("ドメイン層の走査に失敗: %v", err)
	}
	t.Logf("%d ファイルを検査した", checked)
}
