// Package runlogtest は runlog のリポジトリが満たすべき契約を、
// 実装に依らないテストとして持つ。
//
// インメモリと Postgres の2実装から同じ1本を呼ぶ。各実装のテストに書き写すと、
// 片方だけ直したときに「同じ口なのに答えが違う」状態が緑のまま残る
// （accounttest と同じ理由）。
package runlogtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

// Repo は読み書き両方を備えた実装。契約テストは作ってから引く以外に、
// 作ったことを観測する手段が無いので両方を使う。
type Repo interface {
	runlog.Reader
	runlog.Writer
}

var (
	userA = mustUserID("11111111-1111-4111-8111-111111111111")
	userB = mustUserID("22222222-2222-4222-8222-222222222222")
)

func mustUserID(s string) account.UserID {
	id, err := account.NewUserID(s)
	if err != nil {
		panic(err)
	}
	return id
}

func run(t *testing.T, id string, on training.Date, km float64, seconds int) *runlog.RunLog {
	t.Helper()
	l, err := runlog.NewRunLog(runlog.RunLogParams{
		ID: id, PerformedOn: on, DistanceKm: km, DurationSeconds: seconds,
	})
	if err != nil {
		t.Fatalf("NewRunLog: %v", err)
	}
	return l
}

func ids(logs []*runlog.RunLog) []runlog.RunLogID {
	out := make([]runlog.RunLogID, 0, len(logs))
	for _, l := range logs {
		out = append(out, l.ID())
	}
	return out
}

func equalIDs(a, b []runlog.RunLogID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RunContract は runlog.Reader と runlog.Writer の契約を検査する。
// newRepo はケースごとに空の実装を返すこと。
func RunContract(t *testing.T, newRepo func(t *testing.T) Repo) {
	t.Helper()
	ctx := context.Background()
	// MustDate はテストファイルの外で使えない（architecture_test.go）。
	day, err := training.NewDate(2026, time.August, 18)
	if err != nil {
		t.Fatalf("NewDate: %v", err)
	}

	t.Run("保存したものが値ごと読める", func(t *testing.T) {
		r := newRepo(t)
		want := run(t, "a", day, 5.2, 1680)
		if err := r.Save(ctx, userA, want); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := r.FindAll(ctx, userA)
		if err != nil {
			t.Fatalf("FindAll: %v", err)
		}
		if len(got) != 1 || !got[0].Equals(want) {
			t.Fatalf("FindAll = %+v。%+v の1件であること", got, want)
		}
	})

	// クライアントはオフラインで記録して後から送る。タイムアウト後の再送で
	// 重複すると、1回のランが2回に数えられる。
	t.Run("同じ内容の再送は1件のまま成功する", func(t *testing.T) {
		r := newRepo(t)
		for i := range 2 {
			if err := r.Save(ctx, userA, run(t, "a", day, 5.2, 1680)); err != nil {
				t.Fatalf("%d回目の Save: %v", i+1, err)
			}
		}
		got, err := r.FindAll(ctx, userA)
		if err != nil {
			t.Fatalf("FindAll: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("%d件ある。1件のままであること", len(got))
		}
	})

	t.Run("同じIDで内容が違えば衝突にして、元の値を残す", func(t *testing.T) {
		r := newRepo(t)
		first := run(t, "a", day, 5.2, 1680)
		if err := r.Save(ctx, userA, first); err != nil {
			t.Fatalf("Save: %v", err)
		}
		err := r.Save(ctx, userA, run(t, "a", day, 10, 3600))
		if !errors.Is(err, runlog.ErrConflictingRunLog) {
			t.Fatalf("Save = %v。ErrConflictingRunLog であること", err)
		}
		got, _ := r.FindAll(ctx, userA)
		if len(got) != 1 || !got[0].Equals(first) {
			t.Fatalf("FindAll = %+v。最初の値のままであること", got)
		}
	})

	// 一覧の順序が揺れると、同じ記録から違う画面が出る。
	t.Run("実施日の昇順、同じ日はIDの昇順で返す", func(t *testing.T) {
		r := newRepo(t)
		for _, l := range []*runlog.RunLog{
			run(t, "c", day, 3, 900),
			run(t, "b", day.AddDays(-1), 3, 900),
			run(t, "a", day, 3, 900),
		} {
			if err := r.Save(ctx, userA, l); err != nil {
				t.Fatalf("Save: %v", err)
			}
		}
		got, err := r.FindAll(ctx, userA)
		if err != nil {
			t.Fatalf("FindAll: %v", err)
		}
		want := []runlog.RunLogID{"b", "a", "c"}
		if !equalIDs(ids(got), want) {
			t.Fatalf("順序が %v。%v であること", ids(got), want)
		}
	})

	t.Run("消したものは読めず、無いIDを消しても成功する", func(t *testing.T) {
		r := newRepo(t)
		if err := r.Save(ctx, userA, run(t, "a", day, 5, 1500)); err != nil {
			t.Fatalf("Save: %v", err)
		}
		for i := range 2 {
			if err := r.Delete(ctx, userA, "a"); err != nil {
				t.Fatalf("%d回目の Delete: %v", i+1, err)
			}
		}
		got, _ := r.FindAll(ctx, userA)
		if len(got) != 0 {
			t.Fatalf("消したのに %d件ある", len(got))
		}
	})

	// IDはクライアントが採番するので、利用者をまたぐと同じIDがありうる。
	// 衝突にすると他人の記録の存在が分かり、Delete が届くと他人の記録を消せる。
	t.Run("利用者をまたがない", func(t *testing.T) {
		r := newRepo(t)
		mine := run(t, "a", day, 5, 1500)
		if err := r.Save(ctx, userA, mine); err != nil {
			t.Fatalf("Save(userA): %v", err)
		}
		if err := r.Save(ctx, userB, run(t, "a", day, 10, 3000)); err != nil {
			t.Fatalf("別の利用者の同じIDが衝突になった: %v", err)
		}
		if got, _ := r.FindAll(ctx, userB); len(got) != 1 || got[0].Distance().Meters() != 10000 {
			t.Fatalf("userB の記録が %+v。10km の1件であること", got)
		}
		if err := r.Delete(ctx, userB, "a"); err != nil {
			t.Fatalf("Delete(userB): %v", err)
		}
		got, _ := r.FindAll(ctx, userA)
		if len(got) != 1 || !got[0].Equals(mine) {
			t.Fatalf("userB の削除が userA に届いた: %+v", got)
		}
	})
}
