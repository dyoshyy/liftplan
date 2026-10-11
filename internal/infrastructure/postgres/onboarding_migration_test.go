package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// 既存の利用者を、初回の案内を済ませたものとして扱う（0020）。
//
// 済ませていない扱いにすると、使い慣れた人が次に開いたときに設定画面に
// 阻まれる。全部当てたあとで program に行を入れ、0020 をもう一度流す
// （IF NOT EXISTS と ON CONFLICT で何度流しても同じ）。0018 の検査と同じ形。
func TestMigration0020_MarksExistingUsersOnboarded(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	sql, err := os.ReadFile("migrations/0020_onboarding.sql")
	if err != nil {
		t.Fatalf("0020 を読めない: %v", err)
	}

	existing := mustUserID(t, "11111111-1111-4111-8111-111111111111")
	fresh := mustUserID(t, "22222222-2222-4222-8222-222222222222")
	if err := postgres.NewProgramRepository(pool).Save(ctx, existing, samplePrograms(t)); err != nil {
		t.Fatalf("プログラムを保存できない: %v", err)
	}

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0020 が失敗: %v", err)
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
