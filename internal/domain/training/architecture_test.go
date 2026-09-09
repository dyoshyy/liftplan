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

const modulePath = "github.com/dyoshyy/liftplan"

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

		for _, imp := range file.Imports {
			checkImport(t, rel, strings.Trim(imp.Path.Value, `"`))
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

// panic する関数は使える場所を限定する。
//
// 「ここでしか使わない」を規約で守らせると必ず破られるので、AST で機械的に禁止する。
var panickingFunctions = []struct {
	name string
	// allowedFiles に無いファイルから呼ばれたら失敗させる。
	// 空文字は「テストファイルなら許可」を意味する。
	allowedFiles []string
	reason       string
}{
	{
		name:         "MustDate",
		allowedFiles: []string{"date.go", ""},
		reason:       "コンパイル時に確定するリテラル専用。外部入力は ParseDate か FromTime を通すこと",
	},
	{
		name:         "newSlotTemplate",
		allowedFiles: []string{"slot.go"},
		reason:       "カタログ定義専用。実行時の値を渡すとパッケージのロード自体が失敗する",
	},
}

func TestDomain_PanickingFunctionsStayWhereTheyBelong(t *testing.T) {
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

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		checked++

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		isTest := strings.HasSuffix(d.Name(), "_test.go")

		ast.Inspect(file, func(n ast.Node) bool {
			name := calledFunctionName(n)
			if name == "" {
				return true
			}
			for _, fn := range panickingFunctions {
				if fn.name != name {
					continue
				}
				if allowedIn(fn.allowedFiles, d.Name(), isTest) {
					continue
				}
				t.Errorf("%s:%d: %s をここで使ってはいけない。%s",
					rel, fset.Position(n.Pos()).Line, name, fn.reason)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("ドメイン層の走査に失敗: %v", err)
	}
	if checked == 0 {
		t.Fatal("走査対象が0件。検査が空振りしている")
	}
	t.Logf("%d ファイルを検査した", checked)
}

// calledFunctionName は呼び出し式から関数名を取り出す。
func calledFunctionName(n ast.Node) string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		if fn.Sel != nil {
			return fn.Sel.Name
		}
	}
	return ""
}

func allowedIn(allowed []string, fileName string, isTest bool) bool {
	for _, a := range allowed {
		if a == "" && isTest {
			return true
		}
		if a == fileName {
			return true
		}
	}
	return false
}

// checkImport はドメイン層の1つの import を検査する。
//
// 見るのは依存の向きだけで、標準ライブラリは制限しない（D-118）。
// os や net/http が入りうることは承知のうえで、それは規約で守る。
func checkImport(t *testing.T, file, importPath string) {
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
	}
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

// ドメインの値は生成後に状態を変えない。
//
// 実績は「唯一の真実」であり、書き換わると過去のセッションの導出結果まで変わる。
// 値渡しのテストでは「セッターを生やしても通ってしまう」ため検出できない。
//
// 検査対象は次の2つ。
//   - ポインタレシーバのフィールド代入（呼び出し側に直接波及する）
//   - 値レシーバでも添字経由の代入（内部のマップやスライスを共有しているため波及する）
//
// 値レシーバへの平フィールド代入だけは許す。コピーを変えるだけなので
// 波及せず、「新しい値を返す」ビルダーで正当に使われる。
func TestDomain_MethodsDoNotMutateReceiver(t *testing.T) {
	root := domainRoot(t)
	fset := token.NewFileSet()
	checked := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Body == nil {
				continue
			}
			// 名前の無いレシーバ（func (Type) M()）は状態を触れない。
			names := fn.Recv.List[0].Names
			if len(names) == 0 || names[0].Name == "_" {
				continue
			}
			_, isPointer := fn.Recv.List[0].Type.(*ast.StarExpr)
			receiver := names[0].Name
			checked++

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				assign, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range assign.Lhs {
					indexed, ok := mutatesReceiver(lhs, receiver)
					if !ok {
						continue
					}
					// 値レシーバへの平フィールド代入はコピーを変えるだけで
					// 呼び出し側に波及しない。「新しい値を返す」ビルダーで
					// 正当に使われるので許す。
					//
					// ただし r.field[k] = x は、値レシーバでも内部のマップや
					// スライスを共有しているため呼び出し側に波及する。
					if !isPointer && !indexed {
						continue
					}
					t.Errorf("%s:%d: メソッド %s がレシーバの状態を変更している。"+
						"ドメインの値は生成後に変わってはいけない",
						rel, fset.Position(assign.Pos()).Line, fn.Name.Name)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ドメイン層の走査に失敗: %v", err)
	}
	if checked == 0 {
		t.Fatal("レシーバ付きのメソッドを1つも見つけられなかった。検査が空振りしている")
	}
	t.Logf("%d メソッドを検査した", checked)
}

// mutatesReceiver は代入先がレシーバのフィールドかどうか。
// 2つ目の戻り値が、添字経由（r.field[k] = x）かどうかを表す。
func mutatesReceiver(lhs ast.Expr, receiver string) (indexed, ok bool) {
	if index, isIndex := lhs.(*ast.IndexExpr); isIndex {
		lhs, indexed = index.X, true
	}
	sel, isSelector := lhs.(*ast.SelectorExpr)
	if !isSelector {
		return false, false
	}
	ident, isIdent := sel.X.(*ast.Ident)
	return indexed, isIdent && ident.Name == receiver
}
