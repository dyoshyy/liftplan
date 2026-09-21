package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

func configureInput(t *testing.T) usecase.ConfigureProgramInput {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	sets := map[training.MuscleRegion]float64{}
	for _, r := range target.Regions() {
		sets[r] = target.Sets(r)
	}
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	return usecase.ConfigureProgramInput{
		PerWeek: 3, Target: sets, Selected: selected,
		Declared: []exercise.ExerciseID{"bench", "squat", "deadlift"},
	}
}

func newConfigure(t *testing.T, programs *fakeProgram) *usecase.ConfigureProgram {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewConfigureProgram(&fakeExercises{all: pool}, programs)
}

func TestConfigureProgram_SavesTheProgram(t *testing.T) {
	programs := &fakeProgram{}
	if err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), configureInput(t)); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if programs.savedProgram() == nil {
		t.Fatal("プログラムが保存されていない")
	}
	if got := programs.savedProgram().Frequency().PerWeek(); got != 3 {
		t.Errorf("頻度が保存されていない: %d", got)
	}
	if !programs.savedProgram().Includes("bench") {
		t.Error("選択した種目が保存されていない")
	}
}

// 種目マスタに無いIDを黙って受け入れてはいけない。
// 受け入れると SessionPlanner が黙って落とし、ユーザーが選んだ種目が
// 理由の説明なくメニューから消える。
func TestConfigureProgram_RejectsUnknownExercise(t *testing.T) {
	programs := &fakeProgram{}
	in := configureInput(t)
	in.Selected = append(in.Selected, "存在しない種目")

	err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), in)
	if !errors.Is(err, exercise.ErrExerciseNotFound) {
		t.Errorf("未知の種目が弾かれていない: %v", err)
	}
	if programs.savedProgram() != nil {
		t.Error("検証に失敗したのに保存された")
	}
}

func TestConfigureProgram_RejectsInvalidFrequency(t *testing.T) {
	programs := &fakeProgram{}
	in := configureInput(t)
	in.PerWeek = 8

	if err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), in); err == nil {
		t.Error("範囲外の頻度が通った")
	}
	if programs.savedProgram() != nil {
		t.Error("検証に失敗したのに保存された")
	}
}

func TestConfigureProgram_PropagatesSaveError(t *testing.T) {
	boom := errors.New("書けない")
	programs := &fakeProgram{err: boom}

	if err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), configureInput(t)); !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
}

// 種目の取得に失敗したら、検証できないので保存もしない。
func TestConfigureProgram_DoesNotSaveWhenExercisesAreUnavailable(t *testing.T) {
	boom := errors.New("読めない")
	programs := &fakeProgram{}
	uc := usecase.NewConfigureProgram(&fakeExercises{err: boom}, programs)

	if err := uc.Execute(context.Background(), account.DefaultUserID(), configureInput(t)); !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
	if programs.savedProgram() != nil {
		t.Error("種目マスタが読めないのに保存された")
	}
}

// 宣言ゼロの設定を受理してはいけない。
// 受理すると保存は成功するのに、以後すべてのセッション導出が失敗し続ける。
//
// 以前は「選択に KindMain が1つも無い」を、このユースケースが種目マスタと
// 突合して弾いていた。宣言を Program が持つようになって集約が自分で守れる
// ようになったので、判定はドメインへ移った（D-117）。ここが見るのは
// 「集約のエラーが ErrInvalidInput として分類されて返ること」だけ。
func TestConfigureProgram_RejectsSelectionWithoutDeclared(t *testing.T) {
	in := configureInput(t)
	in.Declared = nil

	programs := &fakeProgram{}
	err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), in)
	if !errors.Is(err, program.ErrNoDeclaredExercise) {
		t.Errorf("宣言ゼロが弾かれていない: %v", err)
	}
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("入力の不正として分類されていない: %v", err)
	}
	if programs.savedProgram() != nil {
		t.Error("検証に失敗したのに保存された")
	}
}

// 設定が通った以上、そのプログラムでセッションが導出できること。
// 保存できるのに使えない設定を作らせてはいけない。
func TestConfigureProgram_ProducesAUsableProgram(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	programs := &fakeProgram{}
	if err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), configureInput(t)); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}

	uc := usecase.NewGetSession(
		&fakeExercises{all: pool},
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: programs.savedProgram()},
		planning.DefaultSessionPlanner(),
	)
	s, err := uc.Execute(context.Background(), account.DefaultUserID(), usecase.GetSessionInput{Date: testDate})
	if err != nil {
		t.Fatalf("設定したプログラムでセッションが導出できない: %v", err)
	}
	if len(s.Main()) == 0 {
		t.Error("メイン種目が1つも出ていない")
	}
}

// 入力の不正と I/O 障害が区別できること。
// 区別できないと、プレゼンテーション層は入力ミスを全部 500 にするか
// 障害を全部 400 にするかの二択になる。
func TestConfigureProgram_ClassifiesInvalidInput(t *testing.T) {
	pool, _ := seed.Exercises()

	invalid := map[string]func(in *usecase.ConfigureProgramInput){
		"頻度が範囲外":  func(in *usecase.ConfigureProgramInput) { in.PerWeek = 99 },
		"週目標が空":   func(in *usecase.ConfigureProgramInput) { in.Target = nil },
		"未知の筋区分":  func(in *usecase.ConfigureProgramInput) { in.Target = map[training.MuscleRegion]float64{"膝の皿": 8} },
		"選択が空":    func(in *usecase.ConfigureProgramInput) { in.Selected = nil },
		"種目の重複":   func(in *usecase.ConfigureProgramInput) { in.Selected = append(in.Selected, in.Selected[0]) },
		"実在しない種目": func(in *usecase.ConfigureProgramInput) { in.Selected = append(in.Selected, "無い種目") },
		"メイン種目ゼロ": func(in *usecase.ConfigureProgramInput) { in.Selected = []exercise.ExerciseID{"barbell_curl"} },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			in := configureInput(t)
			mutate(&in)
			err := newConfigure(t, &fakeProgram{}).Execute(context.Background(), account.DefaultUserID(), in)
			if !errors.Is(err, apperror.ErrInvalidInput) {
				t.Errorf("入力の不正として分類されていない: %v", err)
			}
		})
	}

	// I/O 障害は入力の不正として分類してはいけない。
	boom := errors.New("DB接続が切れた")
	for name, uc := range map[string]*usecase.ConfigureProgram{
		"種目マスタの障害": usecase.NewConfigureProgram(&fakeExercises{err: boom}, &fakeProgram{}),
		"保存の障害":    usecase.NewConfigureProgram(&fakeExercises{all: pool}, &fakeProgram{err: boom}),
	} {
		t.Run(name, func(t *testing.T) {
			err := uc.Execute(context.Background(), account.DefaultUserID(), configureInput(t))
			if errors.Is(err, apperror.ErrInvalidInput) {
				t.Errorf("I/O 障害が入力の不正として分類された: %v", err)
			}
			if !errors.Is(err, boom) {
				t.Errorf("元のエラーが辿れない: %v", err)
			}
		})
	}
}

// I/O を必要としない検証は、種目マスタが落ちていても入力の不正として返ること。
func TestConfigureProgram_ValidatesInputBeforeTouchingIO(t *testing.T) {
	exercises := &fakeExercises{err: errors.New("種目テーブルが落ちている")}
	uc := usecase.NewConfigureProgram(exercises, &fakeProgram{})

	in := configureInput(t)
	in.PerWeek = 99
	err := uc.Execute(context.Background(), account.DefaultUserID(), in)
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("I/O 障害に隠れて入力の不正が診断できない: %v", err)
	}
	if exercises.callCount() != 0 {
		t.Errorf("自明な入力ミスなのに I/O を叩いた: %d回", exercises.callCount())
	}
}

// 週目標とまったく噛み合わない選択を受理してはいけない。
// 受理すると補助種目が毎回ゼロになり、設定した週目標が永久に埋まらない。
func TestConfigureProgram_RejectsSelectionDisjointFromTarget(t *testing.T) {
	in := configureInput(t)
	in.Target = map[training.MuscleRegion]float64{training.Biceps: 12}
	in.Selected = []exercise.ExerciseID{"squat", "calf_raise"}

	programs := &fakeProgram{}
	err := newConfigure(t, programs).Execute(context.Background(), account.DefaultUserID(), in)
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("週目標と噛み合わない選択が通った: %v", err)
	}
	if programs.savedProgram() != nil {
		t.Error("検証に失敗したのに保存された")
	}
}

// nil を含む種目マスタで落ちないこと。
func TestConfigureProgram_SkipsNilExercisesInThePool(t *testing.T) {
	pool, _ := seed.Exercises()
	withNil := append([]*exercise.Exercise{nil}, pool...)

	programs := &fakeProgram{}
	uc := usecase.NewConfigureProgram(&fakeExercises{all: withNil}, programs)
	if err := uc.Execute(context.Background(), account.DefaultUserID(), configureInput(t)); err != nil {
		t.Fatalf("nil 混じりのプールで失敗: %v", err)
	}
	if programs.savedProgram() == nil {
		t.Error("保存されていない")
	}
}

// キャンセル済みの context ではリポジトリを叩かないこと。
func TestConfigureProgram_StopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	programs := &fakeProgram{}
	err := newConfigure(t, programs).Execute(ctx, account.DefaultUserID(), configureInput(t))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("キャンセルが伝わっていない: %v", err)
	}
	if programs.savedProgram() != nil {
		t.Error("キャンセル済みなのに保存された")
	}
}
