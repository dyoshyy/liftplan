package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/dyoshyy/liftplan-server/"

// layerOf はパスから層を判定する。数値が小さいほど内側。
//
// infrastructure と presentation が同じ深さなのは、どちらも外界との境界で、
// 内側から見て等距離にあるから。互いに依存してよいという意味ではない。
func layerOf(pkgPath string) (string, int, bool) {
	switch {
	case strings.Contains(pkgPath, "internal/domain"):
		return "domain", 0, true
	case strings.Contains(pkgPath, "internal/application"):
		return "application", 1, true
	case strings.Contains(pkgPath, "internal/infrastructure"):
		return "infrastructure", 2, true
	case strings.Contains(pkgPath, "internal/presentation"):
		return "presentation", 2, true
	case strings.Contains(pkgPath, "cmd/"):
		return "cmd", 3, true
	}
	return "", 0, false
}

// goFile は走査で見つけた1ファイル。
type goFile struct {
	rel     string
	layer   string
	depth   int
	isTest  bool
	imports []string
}

// collectGoFiles はリポジトリ内の Go ファイルと、そのモジュール内 import を集める。
func collectGoFiles(t *testing.T) []goFile {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("ルートを解決できない: %v", err)
	}

	var out []goFile
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		layer, depth, ok := layerOf(rel)
		if !ok {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		f := goFile{
			rel: rel, layer: layer, depth: depth,
			isTest: strings.HasSuffix(rel, "_test.go"),
		}
		for _, imp := range file.Imports {
			f.imports = append(f.imports, strings.Trim(imp.Path.Value, `"`))
		}
		out = append(out, f)
		return nil
	})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}

	// 走査が壊れて0件になっても緑になるのを防ぐ。
	// 検査を通ったのか、何も見ていないのかを区別できなくする類のバグは
	// 検査そのものを無意味にする。
	if len(out) < 20 {
		t.Fatalf("走査したファイルが少なすぎる: %d件", len(out))
	}
	return out
}

// 依存は常に内向き。外側の層を import してはいけない。
func TestOnion_DependenciesPointInward(t *testing.T) {
	for _, f := range collectGoFiles(t) {
		for _, p := range f.imports {
			if !strings.HasPrefix(p, modulePrefix) {
				continue
			}
			toLayer, toDepth, ok := layerOf(strings.TrimPrefix(p, modulePrefix))
			if !ok {
				continue
			}
			if toDepth > f.depth {
				t.Errorf("%s: %s 層が %s 層に依存している（%s）", f.rel, f.layer, toLayer, p)
			}
		}
	}
}

// infrastructure と presentation は互いに依存してはいけない。
//
// 深さが同じなので内向きの検査では捕まらない。ここを開けておくと、
// HTTP のハンドラがインメモリ実装を直接 import できてしまい、
// 「リポジトリ実装の差し替えが cmd に閉じる」という前提が崩れる。
//
// テストファイルは対象外。テストが具象実装を組み立てるのは正当で、
// むしろインメモリ実装で HTTP 層を検証できることが Onion の狙いそのもの。
func TestOnion_SiblingLayersDoNotDependOnEachOther(t *testing.T) {
	for _, f := range collectGoFiles(t) {
		if f.isTest {
			continue
		}
		for _, p := range f.imports {
			if !strings.HasPrefix(p, modulePrefix) {
				continue
			}
			toLayer, toDepth, ok := layerOf(strings.TrimPrefix(p, modulePrefix))
			if !ok {
				continue
			}
			if toDepth == f.depth && toLayer != f.layer {
				t.Errorf("%s: %s 層が同じ深さの %s 層に依存している（%s）",
					f.rel, f.layer, toLayer, p)
			}
		}
	}
}

// 具象のリポジトリ実装を知ってよいのは cmd だけ。
//
// 上の2つの検査から論理的には導けるが、これは Onion の狙いそのもので、
// 破れたときに何が失われるのかが名前から分かる検査を別に置く。
func TestOnion_OnlyCmdKnowsConcreteRepositories(t *testing.T) {
	const infra = modulePrefix + "internal/infrastructure"

	for _, f := range collectGoFiles(t) {
		if f.isTest || f.layer == "cmd" {
			continue
		}
		for _, p := range f.imports {
			if strings.HasPrefix(p, infra) {
				t.Errorf("%s: %s 層が具象のリポジトリ実装を知っている（%s）", f.rel, f.layer, p)
			}
		}
	}
}

// ドメイン層は外部ライブラリに依存しない。
func TestOnion_DomainHasNoExternalDependency(t *testing.T) {
	root, err := filepath.Abs("./domain")
	if err != nil {
		t.Fatalf("ドメインを解決できない: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("ドメインディレクトリが無い: %v", err)
	}

	seen := 0
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		seen++
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, modulePrefix) {
				continue
			}
			// ドット付きはホスト名を含む＝外部モジュール
			if strings.Contains(strings.Split(p, "/")[0], ".") {
				t.Errorf("%s: ドメイン層が外部ライブラリに依存している: %s", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
	if seen < 20 {
		t.Fatalf("走査したファイルが少なすぎる: %d件", seen)
	}
}
