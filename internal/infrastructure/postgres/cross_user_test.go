package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// 所有者の分離は、リポジトリが守ると決めた契約である。
//
// ここが破れると、上の層は何をしても他人の記録を見せる。ドメインも
// ユースケースも「誰が」を知らないまま UserID を素通しするだけなので、
// 突き合わせる場所はここしかない。
//
// 同じ観点のテストがインメモリ側（cross_user_test.go）にもある。
// ケース名を揃えてあるので、片方だけ通る状態は読んで分かる。
//
// 両方で回すのは、分離のやり方が実装ごとに違うため。インメモリは
// 所有者ごとの map、Postgres は主キーと WHERE 句。片方が守っていても
// もう片方が漏らす経路は、共通のテストが無いと見つからない。

func userA(t *testing.T) account.UserID { return mustUserID(t, "11111111-1111-4111-8111-111111111111") }
func userB(t *testing.T) account.UserID { return mustUserID(t, "22222222-2222-4222-8222-222222222222") }

var day = training.MustDate(2026, time.August, 17)

func mkSetLog(t *testing.T, id string, kg float64) *setlog.SetLog {
	t.Helper()
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: id, PerformedOn: day, ExerciseID: "bench",
		WeightKg: kg, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return l
}

func mustUserID(t *testing.T, s string) account.UserID {
	t.Helper()
	id, err := account.NewUserID(s)
	if err != nil {
		t.Fatalf("UserID が作れない: %v", err)
	}
	return id
}

// 別のユーザーの実績は読みに混ざらない。
func TestSetLogRepository_KeepsUsersApart(t *testing.T) {
	pool := migratedDB(t)
	repo := postgres.NewSetLogRepository(pool)
	ctx := context.Background()
	a, b := userA(t), userB(t)

	if err := repo.Save(ctx, a, []*setlog.SetLog{mkSetLog(t, "a1", 100)}); err != nil {
		t.Fatalf("A の保存に失敗: %v", err)
	}
	if err := repo.Save(ctx, b, []*setlog.SetLog{mkSetLog(t, "b1", 60)}); err != nil {
		t.Fatalf("B の保存に失敗: %v", err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("A の取得に失敗: %v", err)
	}
	for _, l := range got.Logs() {
		if l.ID() == "b1" {
			t.Fatalf("A の履歴に B の実績が混ざっている")
		}
	}
	if len(got.Logs()) != 1 {
		t.Errorf("A の履歴が %d 件。1件のはず", len(got.Logs()))
	}
}

// 同じIDを別のユーザーが使っても衝突にしない。
//
// set_logs.id はクライアントが採番する。衝突として弾くと、
// 「そのIDの記録が存在する」ことが他人に分かる。
func TestSetLogRepository_DoesNotCollideAcrossUsers(t *testing.T) {
	pool := migratedDB(t)
	repo := postgres.NewSetLogRepository(pool)
	ctx := context.Background()
	a, b := userA(t), userB(t)

	if err := repo.Save(ctx, a, []*setlog.SetLog{mkSetLog(t, "same", 100)}); err != nil {
		t.Fatalf("A の保存に失敗: %v", err)
	}

	// 同じIDで内容が違う。同じユーザーなら ErrConflictingSetLog になる組み合わせ。
	if err := repo.Save(ctx, b, []*setlog.SetLog{mkSetLog(t, "same", 60)}); err != nil {
		t.Fatalf("B が同じIDを使えない: %v", err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("A の取得に失敗: %v", err)
	}
	if len(got.Logs()) != 1 || got.Logs()[0].Weight().Kg() != 100 {
		t.Errorf("A の実績が B の保存で書き換わった: %+v", got.Logs())
	}
}

// 別のユーザーのIDを消しても、本人の記録は残る。
func TestSetLogRepository_DeleteDoesNotReachOtherUsers(t *testing.T) {
	pool := migratedDB(t)
	repo := postgres.NewSetLogRepository(pool)
	ctx := context.Background()
	a, b := userA(t), userB(t)

	if err := repo.Save(ctx, a, []*setlog.SetLog{mkSetLog(t, "same", 100)}); err != nil {
		t.Fatalf("A の保存に失敗: %v", err)
	}

	// B から見れば存在しないIDなので、成功として扱われる（契約どおり）。
	if err := repo.Delete(ctx, b, "same"); err != nil {
		t.Fatalf("B の削除が失敗した: %v", err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("A の取得に失敗: %v", err)
	}
	if len(got.Logs()) != 1 {
		t.Errorf("B の削除で A の実績が消えた")
	}
}

// 同じ日付でも、別のユーザーのコンディションは混ざらない。
func TestConditionRepository_KeepsUsersApart(t *testing.T) {
	pool := migratedDB(t)
	repo := postgres.NewConditionRepository(pool)
	ctx := context.Background()
	a, b := userA(t), userB(t)

	if err := repo.Save(ctx, a, []condition.DailyCondition{
		condition.NewDailyCondition(day).WithBodyWeight(70),
	}); err != nil {
		t.Fatalf("A の保存に失敗: %v", err)
	}
	if err := repo.Save(ctx, b, []condition.DailyCondition{
		condition.NewDailyCondition(day).WithBodyWeight(90),
	}); err != nil {
		t.Fatalf("B の保存に失敗: %v", err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("A の取得に失敗: %v", err)
	}
	kg, ok := got.BodyWeightAsOf(day)
	if !ok {
		t.Fatalf("A の体重が読めない")
	}
	if kg != 70 {
		t.Errorf("A の体重が %v kg。B の 90 に上書きされている", kg)
	}
}

// 別のユーザーが足した種目は見えない。
func TestExerciseRepository_ReturnsSeedPlusOwnCustoms(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)
	a, b := newUser(t), newUser(t)

	mine := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, mine); err != nil {
		t.Fatal(err)
	}

	gotA, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatalf("A の取得に失敗: %v", err)
	}
	if len(gotA) != len(seedAll)+1 || gotA[len(gotA)-1].ID() != mine.ID() {
		t.Errorf("A の一覧に自分の種目が末尾に1件足されていない（%d 件）", len(gotA))
	}
	gotB, err := repo.FindAll(ctx, b)
	if err != nil {
		t.Fatalf("B の取得に失敗: %v", err)
	}
	if len(gotB) != len(seedAll) {
		t.Errorf("B に A の種目が見えている（%d 件）", len(gotB))
	}
}

// 別のユーザーの設定は見えない。
func TestProgramRepository_KeepsUsersApart(t *testing.T) {
	pool := migratedDB(t)
	repo := postgres.NewProgramRepository(pool)
	ctx := context.Background()
	a, b := userA(t), userB(t)

	if err := repo.Save(ctx, a, samplePrograms(t)); err != nil {
		t.Fatalf("A の保存に失敗: %v", err)
	}

	// B はまだ設定していない。A の設定が見えてはいけない。
	if _, err := repo.Get(ctx, b); !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("B の取得が %v。ErrProgramNotConfigured のはず", err)
	}
}
