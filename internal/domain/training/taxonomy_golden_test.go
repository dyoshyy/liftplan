package training_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// 列挙の文字列値は DB と JSON にそのまま永続化される。値を変えると既存データが
// どの定数とも一致しなくなり、しかも WeeklyVolumeTarget.Sets は未設定の区分に対して
// エラーではなく 0 を返すため、「目標0セット」として静かに計画され続ける。
//
// 表記規約（大文字・空白なし）を守るだけでは、CHEST_UPPER → CHEST_CLAVICULAR のような
// 規約を保ったままの改名を検出できない。値そのものをリテラルで固定する。
func TestMuscleRegion_GoldenValues(t *testing.T) {
	want := []string{
		"ABS",
		"ADDUCTOR",
		"BICEPS",
		"CALF",
		"CHEST_LOWER",
		"CHEST_MID",
		"CHEST_UPPER",
		"ERECTOR",
		"FOREARM",
		"FRONT_DELT",
		"GLUTE",
		"HAMSTRING",
		"LAT",
		"OBLIQUE",
		"QUAD",
		"REAR_DELT",
		"SIDE_DELT",
		"TRAP_MID",
		"TRAP_UPPER",
		"TRICEPS_LATERAL",
		"TRICEPS_LONG",
	}

	got := make([]string, 0, len(want))
	for _, r := range training.AllMuscleRegions() {
		got = append(got, string(r))
	}
	assertGolden(t, "MuscleRegion", got, want)
}

func TestExerciseKind_GoldenValues(t *testing.T) {
	want := []string{"ACCESSORY", "MAIN"}

	got := make([]string, 0, len(want))
	for _, k := range training.AllExerciseKinds() {
		got = append(got, string(k))
	}
	assertGolden(t, "ExerciseKind", got, want)
}

func assertGolden(t *testing.T, name string, got, want []string) {
	t.Helper()

	inWant := map[string]bool{}
	for _, v := range want {
		inWant[v] = true
	}
	inGot := map[string]bool{}
	for _, v := range got {
		inGot[v] = true
	}

	for _, v := range want {
		if !inGot[v] {
			t.Errorf("%s: 永続化される値 %q が消えている。既存データが読めなくなる", name, v)
		}
	}
	for _, v := range got {
		if !inWant[v] {
			t.Errorf("%s: 未知の値 %q が増えている。意図した追加ならゴールデンに追記すること", name, v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s: 件数が違う: got %d, want %d", name, len(got), len(want))
	}
}

// 定数を宣言したのに一覧へ追加し忘れると、Valid() が false のまま静かに使われる。
// シードがその定数を参照した瞬間、コンパイル時ではなく起動時に落ちる。
// ソースを直接読んで、宣言と一覧の差を検出する。
func TestTaxonomy_EveryDeclaredConstantIsListed(t *testing.T) {
	declared := declaredConstants(t)

	cases := []struct {
		typeName string
		listed   []string
	}{
		{"MuscleRegion", toStrings(training.AllMuscleRegions())},
		{"ExerciseKind", toStrings(training.AllExerciseKinds())},
	}

	for _, c := range cases {
		t.Run(c.typeName, func(t *testing.T) {
			want := declared[c.typeName]
			if len(want) == 0 {
				t.Fatalf("%s の定数宣言を1つも見つけられなかった。検査が空振りしている", c.typeName)
			}

			listed := map[string]bool{}
			for _, v := range c.listed {
				listed[v] = true
			}
			for _, v := range want {
				if !listed[v] {
					t.Errorf("定数 %q が宣言されているのに All%ss() の一覧に無い", v, c.typeName)
				}
			}
			if len(want) != len(c.listed) {
				t.Errorf("宣言数と一覧の件数が違う: 宣言 %d, 一覧 %d", len(want), len(c.listed))
			}
		})
	}
}

// Valid() に「一覧には無いがこの値だけは通す」という特例を足されると、
// 境界では受理されるのに網羅検査からは見えない値が生まれる。
// SessionPlanner の分岐は3つの種別を前提に書かれるため、そうした値は
// どのバケツにも入らないまま静かに落ちる。
//
// 特例は必ずソース上の文字列リテラルとして現れる。taxonomy.go の全リテラルを
// 集め、Valid() が true を返すものが一覧に載っていることを確かめる。
func TestTaxonomy_ValidAgreesWithTheList(t *testing.T) {
	literals := stringLiterals(t)
	if len(literals) == 0 {
		t.Fatal("taxonomy.go から文字列リテラルを1つも拾えなかった。検査が空振りしている")
	}

	regions := lookupOf(toStrings(training.AllMuscleRegions()))
	kinds := lookupOf(toStrings(training.AllExerciseKinds()))

	for _, lit := range literals {
		if training.MuscleRegion(lit).Valid() && !regions[lit] {
			t.Errorf("MuscleRegion(%q) が Valid だが一覧に無い。Valid に特例が入っている", lit)
		}
		if training.ExerciseKind(lit).Valid() && !kinds[lit] {
			t.Errorf("ExerciseKind(%q) が Valid だが一覧に無い。Valid に特例が入っている", lit)
		}
	}
}

func lookupOf(values []string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// stringLiterals は taxonomy.go に現れるすべての文字列リテラルを返す。
func stringLiterals(t *testing.T) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), taxonomySourcePath(t), nil, 0)
	if err != nil {
		t.Fatalf("taxonomy.go をパースできない: %v", err)
	}

	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			out = append(out, mustUnquote(t, lit.Value))
		}
		return true
	})
	return out
}

func taxonomySourcePath(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("テストファイルの位置を解決できない")
	}
	return filepath.Join(filepath.Dir(thisFile), "taxonomy.go")
}

// declaredConstants は taxonomy.go の const 宣言を型ごとに読み取る。
func declaredConstants(t *testing.T) map[string][]string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), taxonomySourcePath(t), nil, 0)
	if err != nil {
		t.Fatalf("taxonomy.go をパースできない: %v", err)
	}

	out := map[string][]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			typeName, ok := value.Type.(*ast.Ident)
			if !ok {
				continue
			}
			for _, v := range value.Values {
				lit, ok := v.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				out[typeName.Name] = append(out[typeName.Name], mustUnquote(t, lit.Value))
			}
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

func mustUnquote(t *testing.T, s string) string {
	t.Helper()
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		t.Fatalf("文字列リテラルとして解釈できない: %s", s)
	}
	return s[1 : len(s)-1]
}

func toStrings[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, string(v))
	}
	sort.Strings(out)
	return out
}
