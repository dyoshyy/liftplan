package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// 既存の利用者を、初回の案内を済ませたものとして扱う（0021）。
//
// 済ませていない扱いにすると、使い慣れた人が次に開いたときに設定画面に
// 阻まれる。全部当てたあとで program に行を入れ、0021 をもう一度流す
// （IF NOT EXISTS と ON CONFLICT で何度流しても同じ）。0018 の検査と同じ形。
func TestMigration0021_MarksExistingUsersOnboarded(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	sql, err := os.ReadFile("migrations/0021_onboarding.sql")
	if err != nil {
		t.Fatalf("0021 を読めない: %v", err)
	}

	existing := mustUserID(t, "11111111-1111-4111-8111-111111111111")
	fresh := mustUserID(t, "22222222-2222-4222-8222-222222222222")
	if err := postgres.NewProgramRepository(pool).Save(ctx, existing, samplePrograms(t)); err != nil {
		t.Fatalf("プログラムを保存できない: %v", err)
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0021 が失敗: %v", err)
	}

	repo := postgres.NewOnboardingRepository(pool)
	for _, c := range []struct {
		name string
		user account.UserID
		want bool
	}{
		{"program がある利用者は済み", existing, true},
		{"program が無い利用者はまだ", fresh, false},
	} {
		got, err := repo.OnboardingCompleted(ctx, c.user)
		if err != nil || got != c.want {
			t.Errorf("%s: %v, %v（期待 %v）", c.name, got, err, c.want)
		}
	}
}

// 2回目の完了は最初の時刻を上書きしない。
//
// 画面は送信に失敗すると最初から送り直すので、完了は何度も届きうる。
// 上書きすると「いつ始めた人か」が最後の送り直しの時刻にずれる。
// インメモリ実装は時刻を外から読めないので、契約ではなくここで見る。
func TestOnboardingRepository_KeepsFirstCompletion(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	user := mustUserID(t, "11111111-1111-4111-8111-111111111111")
	first := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)

	repo := postgres.NewOnboardingRepository(pool)
	for _, at := range []time.Time{first, first.Add(time.Hour)} {
		if err := repo.CompleteOnboarding(ctx, user, at); err != nil {
			t.Fatalf("CompleteOnboarding(%v): %v", at, err)
		}
	}

	var got time.Time
	if err := pool.QueryRow(ctx,
		"SELECT completed_at FROM onboarding WHERE user_id = $1", user.String(),
	).Scan(&got); err != nil {
		t.Fatalf("completed_at を読めない: %v", err)
	}
	if !got.Equal(first) {
		t.Errorf("completed_at = %v（期待 %v）", got, first)
	}
}
