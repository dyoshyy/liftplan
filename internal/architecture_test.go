package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/dyoshyy/liftplan-server/"

// layer は Onion の1層。depth が小さいほど内側。
type layer struct {
	name  string
	depth int
}

// layers は許可された層の一覧。プレフィックスはディレクトリ境界で照合する。
//
// 未知のパッケージを「分類対象外」として黙って無視すると、その1枚を
// 挟むだけで全ての検査が素通りする。許可リスト方式にして、
// 新しいディレクトリを作った人に層の宣言を強制する。
var layers = []struct {
	prefix string
	layer  layer
}{
	{"internal/domain", layer{"domain", 0}},
	{"internal/application", layer{"application", 1}},
	{"internal/infrastructure", layer{"infrastructure", 2}},
	{"internal/presentation", layer{"presentation", 2}},
	{"cmd", layer{"cmd", 3}},
}

// exemptDirs は層を持たなくてよいディレクトリ。
//
// internal 直下にはこの検査そのものしか置かない。層を持つコードを
// 置き始めたら、それは層を宣言すべきものなので、ここを増やさないこと。
var exemptDirs = map[string]bool{".": true, "internal": true}

// layerOf はスラッシュ区切りのパッケージパスから層を判定する。
//
// strings.Contains を使わないのは、internal/domainsql や internal/infracmd の
// ような名前が別の層として誤分類されるため。ディレクトリ境界で照合する。
func layerOf(pkgPath string) (layer, bool) {
	pkgPath = strings.TrimSuffix(pkgPath, "/")
	for _, l := range layers {
		if pkgPath == l.prefix || strings.HasPrefix(pkgPath, l.prefix+"/") {
			return l.layer, true
		}
	}
	return layer{}, false
}

// goFile は走査で見つけた1ファイル。
type goFile struct {
	rel     string
	layer   layer
	isTest  bool
	imports []string
}

// skipDir は Go ツールチェーンが無視するディレクトリ。
//
// testdata の中身はコンパイルされないので、そこに置かれたコードを
// 違反として報告すると偽陽性になる。
func skipDir(name string) bool {
	return name == ".git" || name == "testdata" ||
		strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// collectGoFiles はリポジトリ内の Go ファイルと、そのモジュール内 import を集める。
//
// 第2の戻り値は、どの層にも属さないパッケージのパス。
func collectGoFiles(t *testing.T) ([]goFile, []string) {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("ルートを解決できない: %v", err)
	}

	var (
		out         []goFile
		unclassered = map[string]bool{}
	)
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
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

		dir := filepath.ToSlash(filepath.Dir(rel))
		l, ok := layerOf(dir)
		if !ok {
			if !exemptDirs[dir] {
				unclassered[dir] = true
			}
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		f := goFile{rel: rel, layer: l, isTest: strings.HasSuffix(rel, "_test.go")}
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

	dirs := make([]string, 0, len(unclassered))
	for d := range unclassered {
		dirs = append(dirs, d)
	}
	return out, dirs
}

// violations は依存方向の違反を集める。
//
// 検査本体を純粋な関数にしておくのは、既知の違反を食わせて
// 「検査が実際に落ちること」を確かめられるようにするため。
// 検査が静かに検出力を失っても、下限件数のような粗い保険では気づけない。
func violations(files []goFile) []string {
	var out []string
	for _, f := range files {
		for _, p := range f.imports {
			if !strings.HasPrefix(p, modulePrefix) {
				continue
			}
			to, ok := layerOf(strings.TrimPrefix(p, modulePrefix))
			if !ok {
				continue
			}
			switch {
			case to.depth > f.layer.depth:
				out = append(out, f.rel+": "+f.layer.name+" 層が外側の "+to.name+" 層に依存している（"+p+"）")
			case to.depth == f.layer.depth && to.name != f.layer.name && !f.isTest:
				// テストファイルは対象外。テストが具象実装を組み立てるのは
				// 正当で、むしろインメモリ実装で HTTP 層を検証できること自体が
				// Onion の狙いそのもの。
				out = append(out, f.rel+": "+f.layer.name+" 層が同じ深さの "+to.name+" 層に依存している（"+p+"）")
			}
		}
	}
	return out
}

// すべてのパッケージが層を宣言していること。
//
// 分類対象外のパッケージを1枚挟むと、その先の依存が検査から消える。
// 実際、internal/glue のような中継を置くだけで presentation から
// infrastructure を掴めてしまう。
func TestOnion_EveryPackageDeclaresItsLayer(t *testing.T) {
	_, unclassified := collectGoFiles(t)
	for _, dir := range unclassified {
		t.Errorf("%s はどの層にも属していない。層を決めて layers に追加すること", dir)
	}
}

// 依存は常に内向き。外側の層と、同じ深さの別の層を import してはいけない。
//
// infrastructure と presentation の深さが同じなのは、どちらも外界との
// 境界で内側から見て等距離だから。互いに依存してよいという意味ではない。
// ここを開けておくと HTTP のハンドラがインメモリ実装を直接 import でき、
// 「リポジトリ実装の差し替えが cmd に閉じる」という前提が崩れる。
func TestOnion_DependenciesPointInward(t *testing.T) {
	files, _ := collectGoFiles(t)
	for _, v := range violations(files) {
		t.Error(v)
	}
}

// 検査そのものが検出力を持っていること（陽性コントロール）。
//
// 実装をゆるめても、違反が無ければ検査は緑のままになる。既知の違反を
// 食わせて落ちることを確かめないと、静かに無力化された検査を
// 「通っている」と読んでしまう。
func TestOnion_TheCheckItselfDetectsViolations(t *testing.T) {
	cases := map[string]goFile{
		"domain が application に依存": {
			rel: "internal/domain/x.go", layer: layer{"domain", 0},
			imports: []string{modulePrefix + "internal/application/usecase"},
		},
		"application が infrastructure に依存": {
			rel: "internal/application/x.go", layer: layer{"application", 1},
			imports: []string{modulePrefix + "internal/infrastructure/memory"},
		},
		"presentation が infrastructure に依存": {
			rel: "internal/presentation/x.go", layer: layer{"presentation", 2},
			imports: []string{modulePrefix + "internal/infrastructure/memory"},
		},
		"infrastructure が presentation に依存": {
			rel: "internal/infrastructure/x.go", layer: layer{"infrastructure", 2},
			imports: []string{modulePrefix + "internal/presentation/httpapi"},
		},
		"domain が cmd に依存": {
			rel: "internal/domain/x.go", layer: layer{"domain", 0},
			imports: []string{modulePrefix + "cmd/api"},
		},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if got := violations([]goFile{f}); len(got) == 0 {
				t.Error("違反を検出できていない")
			}
		})
	}

	// 正当な依存を違反と report しないこと。
	legit := []goFile{
		{rel: "internal/application/x.go", layer: layer{"application", 1},
			imports: []string{modulePrefix + "internal/domain/training"}},
		{rel: "cmd/api/main.go", layer: layer{"cmd", 3},
			imports: []string{modulePrefix + "internal/infrastructure/memory"}},
		{rel: "internal/presentation/x_test.go", layer: layer{"presentation", 2}, isTest: true,
			imports: []string{modulePrefix + "internal/infrastructure/memory"}},
	}
	if got := violations(legit); len(got) != 0 {
		t.Errorf("正当な依存を違反と判定した: %v", got)
	}
}

// 名前が紛らわしいパスを取り違えないこと。
func TestLayerOf_MatchesOnDirectoryBoundaries(t *testing.T) {
	classified := map[string]string{
		"internal/domain":                "domain",
		"internal/domain/training/seed":  "domain",
		"internal/application/usecase":   "application",
		"internal/infrastructure/memory": "infrastructure",
		"internal/presentation/httpapi":  "presentation",
		"cmd/api":                        "cmd",
	}
	for path, want := range classified {
		got, ok := layerOf(path)
		if !ok || got.name != want {
			t.Errorf("%s: %q と判定された（期待 %q）", path, got.name, want)
		}
	}

	// 部分文字列一致だと層を偽装できるパス。
	for _, path := range []string{
		"internal/domainsql", "internal/infracmd", "internal/glue",
		"tools/cmd", "pkg/adapter", "internal/domain2",
	} {
		if l, ok := layerOf(path); ok {
			t.Errorf("%s が %s 層として通った", path, l.name)
		}
	}
}

// 具象のリポジトリ実装を知ってよいのは cmd だけ。
//
// 上の検査から論理的には導けるが、これは Onion の狙いそのもので、
// 破れたときに何が失われるのかが名前から分かる検査を別に置く。
func TestOnion_OnlyCmdKnowsConcreteRepositories(t *testing.T) {
	const infra = modulePrefix + "internal/infrastructure"

	files, _ := collectGoFiles(t)
	for _, f := range files {
		if f.isTest || f.layer.name == "cmd" {
			continue
		}
		for _, p := range f.imports {
			if strings.HasPrefix(p, infra) {
				t.Errorf("%s: %s 層が具象のリポジトリ実装を知っている（%s）", f.rel, f.layer.name, p)
			}
		}
	}
}
