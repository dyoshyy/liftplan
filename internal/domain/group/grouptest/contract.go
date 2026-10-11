// Package grouptest は group のリポジトリと読み口が満たすべき契約を、
// 実装に依らないテストとして持つ。
//
// インメモリと Postgres の両方から同じ1本を呼ぶ（accounttest と同じ理由）。
// 片方だけ直したときに「同じ口なのに答えが違う」状態が緑のまま残らない。
package grouptest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// GroupRepo は読み書き両方を備えたグループの実装。
type GroupRepo interface {
	group.Reader
	group.Writer
}

// ExerciseRepo は種目の読み書き。名前を付け替える・消すのに使う。
type ExerciseRepo interface {
	exercise.Reader
	exercise.Writer
}

// Repos は1つのテストケースで使う実装の組。
//
// 記録と種目は、仲間の記録の読み口に材料を入れるために要る。読み口は
// それらと同じ保存先を見ている必要がある（Postgres なら同じスキーマ）。
type Repos struct {
	Groups    GroupRepo
	Activity  group.ActivityReader
	SetLogs   setlog.Writer
	Exercises ExerciseRepo
}

// Seed は契約で使うプリセット。実装を組むときにこれを渡すこと。
//
// 2つだけにするのは、名前の引き方を見るのに十分で、読む人が数えられるから。
func Seed() []*exercise.Exercise {
	return []*exercise.Exercise{
		mustExercise(exercise.ExerciseParams{ID: "bench", Name: "ベンチプレス"}),
		mustExercise(exercise.ExerciseParams{ID: "squat", Name: "スクワット"}),
	}
}

var (
	// Postgres の timestamptz はマイクロ秒までなので、ナノ秒は持たせない。
	at = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	userA = mustUserID("11111111-1111-4111-8111-111111111111")
	userB = mustUserID("22222222-2222-4222-8222-222222222222")
	userC = mustUserID("33333333-3333-4333-8333-333333333333")
)

// RunGroupContract はグループの読み書きの契約を検査する。
//
// newRepos はケースごとに空の実装を返すこと。
func RunGroupContract(t *testing.T, newRepos func(t *testing.T) Repos) {
	t.Helper()
	ctx := context.Background()

	// 値まで見る。Find が何を返しても「無いものは引けない」側は緑になる。
	t.Run("作ったものを ID でもトークンでも引ける", func(t *testing.T) {
		r := newRepos(t).Groups
		g := newGroup(t, "ジム仲間", member(t, userA, "よし", at), member(t, userB, "たろう", at.Add(time.Minute)))
		create(t, r, g)

		for name, find := range map[string]func() (*group.Group, error){
			"ID":   func() (*group.Group, error) { return r.Find(ctx, g.ID()) },
			"トークン": func() (*group.Group, error) { return r.FindByInvite(ctx, g.Invite()) },
		} {
			got, err := find()
			if err != nil {
				t.Fatalf("%sで引けない: %v", name, err)
			}
			assertSameGroup(t, got, g)
		}
	})

	t.Run("無いグループは ErrGroupNotFound", func(t *testing.T) {
		r := newRepos(t).Groups
		id, _ := group.NewRandomID()
		if _, err := r.Find(ctx, id); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("ID: %v。ErrGroupNotFound のはず", err)
		}
		token, _ := group.NewInviteToken()
		if _, err := r.FindByInvite(ctx, token); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("トークン: %v。ErrGroupNotFound のはず", err)
		}
	})

	// 作り直したのに古いリンクで入れると、漏れたリンクを止める手段が無い。
	t.Run("作り直した後は古いトークンで引けない", func(t *testing.T) {
		r := newRepos(t).Groups
		g := newGroup(t, "ジム仲間", member(t, userA, "よし", at))
		create(t, r, g)
		old := g.Invite()
		next, _ := group.NewInviteToken()

		if err := r.ReplaceInvite(ctx, g.ID(), next); err != nil {
			t.Fatalf("差し替えに失敗: %v", err)
		}
		if _, err := r.FindByInvite(ctx, old); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("古いトークンで: %v。ErrGroupNotFound のはず", err)
		}
		got, err := r.FindByInvite(ctx, next)
		if err != nil {
			t.Fatalf("新しいトークンで引けない: %v", err)
		}
		if got.ID() != g.ID() {
			t.Errorf("別のグループが返った: %s", got.ID())
		}
	})

	t.Run("無いグループのトークンは差し替えられない", func(t *testing.T) {
		r := newRepos(t).Groups
		id, _ := group.NewRandomID()
		next, _ := group.NewInviteToken()
		if err := r.ReplaceInvite(ctx, id, next); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("%v。ErrGroupNotFound のはず", err)
		}
	})

	// 二度押し・通信のやり直しで2行にならず、表示名も最初のまま。
	t.Run("同じ人を二度足しても1人で、表示名は最初のまま", func(t *testing.T) {
		r := newRepos(t).Groups
		g := newGroup(t, "ジム仲間", member(t, userA, "よし", at))
		create(t, r, g)

		if err := r.AddMember(ctx, g.ID(), member(t, userB, "たろう", at.Add(time.Minute))); err != nil {
			t.Fatalf("1回目: %v", err)
		}
		if err := r.AddMember(ctx, g.ID(), member(t, userB, "別名", at.Add(time.Hour))); err != nil {
			t.Fatalf("2回目: %v", err)
		}
		got := find(t, r, g.ID())
		ms := got.Members()
		if len(ms) != 2 {
			t.Fatalf("メンバーが %d 人。2人のはず", len(ms))
		}
		if ms[1].UserID() != userB || ms[1].DisplayName().String() != "たろう" {
			t.Errorf("2人目が %s/%s。B/たろう のはず", ms[1].UserID(), ms[1].DisplayName())
		}
	})

	t.Run("無いグループには足せない", func(t *testing.T) {
		r := newRepos(t).Groups
		id, _ := group.NewRandomID()
		if err := r.AddMember(ctx, id, member(t, userA, "よし", at)); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("%v。ErrGroupNotFound のはず", err)
		}
	})

	t.Run("外した人はメンバーから消え、いない人を外しても成功する", func(t *testing.T) {
		r := newRepos(t).Groups
		g := newGroup(t, "ジム仲間", member(t, userA, "よし", at), member(t, userB, "たろう", at.Add(time.Minute)))
		create(t, r, g)

		if err := r.RemoveMember(ctx, g.ID(), userB); err != nil {
			t.Fatalf("外せない: %v", err)
		}
		if err := r.RemoveMember(ctx, g.ID(), userC); err != nil {
			t.Fatalf("いない人で失敗した: %v", err)
		}
		got := find(t, r, g.ID())
		if ids := memberIDs(got); !slices.Equal(ids, []account.UserID{userA}) {
			t.Errorf("メンバーが %v。[A] のはず", ids)
		}
	})

	t.Run("空のグループは消え、二度消しても成功する", func(t *testing.T) {
		r := newRepos(t).Groups
		g := newGroup(t, "ジム仲間", member(t, userA, "よし", at))
		create(t, r, g)
		if err := r.RemoveMember(ctx, g.ID(), userA); err != nil {
			t.Fatal(err)
		}

		if err := r.DeleteIfEmpty(ctx, g.ID()); err != nil {
			t.Fatalf("消せない: %v", err)
		}
		if err := r.DeleteIfEmpty(ctx, g.ID()); err != nil {
			t.Fatalf("二度目で失敗した: %v", err)
		}
		if _, err := r.Find(ctx, g.ID()); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("ID で: %v。ErrGroupNotFound のはず", err)
		}
		if _, err := r.FindByInvite(ctx, g.Invite()); !errors.Is(err, group.ErrGroupNotFound) {
			t.Errorf("トークンで: %v。ErrGroupNotFound のはず", err)
		}
	})

	// 最後の1人が抜けるのと同時に誰かが参加したとき。抜けた側は空だと
	// 判断して消しに来るが、その間に入った人ごと消すと、参加できたのに
	// 入っていない人ができる。
	t.Run("メンバーが残っていれば消さない", func(t *testing.T) {
		r := newRepos(t).Groups
		g := newGroup(t, "ジム仲間", member(t, userA, "よし", at))
		create(t, r, g)
		if err := r.AddMember(ctx, g.ID(), member(t, userB, "たろう", at.Add(time.Minute))); err != nil {
			t.Fatal(err)
		}
		if err := r.RemoveMember(ctx, g.ID(), userA); err != nil {
			t.Fatal(err)
		}

		if err := r.DeleteIfEmpty(ctx, g.ID()); err != nil {
			t.Fatalf("失敗した: %v", err)
		}
		got := find(t, r, g.ID())
		if ids := memberIDs(got); !slices.Equal(ids, []account.UserID{userB}) {
			t.Errorf("メンバーが %v。[B] のはず", ids)
		}
	})

	// 一覧に他人のグループが混ざると、名前と ID が漏れる。
	t.Run("一覧は自分がメンバーのグループだけを作った順に返す", func(t *testing.T) {
		r := newRepos(t).Groups
		first := newGroupAt(t, "ジム仲間", at, member(t, userA, "よし", at))
		others := newGroupAt(t, "他人だけ", at.Add(time.Minute), member(t, userB, "たろう", at))
		second := newGroupAt(t, "会社", at.Add(2*time.Minute), member(t, userB, "田中", at), member(t, userA, "吉田", at.Add(time.Hour)))
		create(t, r, second)
		create(t, r, others)
		create(t, r, first)

		got, err := r.ListByMember(ctx, userA)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, g := range got {
			names = append(names, g.Name().String())
		}
		if !slices.Equal(names, []string{"ジム仲間", "会社"}) {
			t.Errorf("一覧が %v。[ジム仲間 会社] のはず", names)
		}
		// 一覧から引いたグループも、メンバーまで揃っていること。
		// VisibleTo はここに載ったメンバーで判断する。
		if len(got) == 2 {
			if ids := memberIDs(got[1]); !slices.Equal(ids, []account.UserID{userB, userA}) {
				t.Errorf("会社のメンバーが %v。[B A] のはず", ids)
			}
		}

		none, err := r.ListByMember(ctx, userC)
		if err != nil {
			t.Fatal(err)
		}
		if none == nil || len(none) != 0 {
			t.Errorf("どこにも入っていない人の一覧が %v。空のスライスのはず", none)
		}
	})
}

// RunActivityContract は仲間の記録の読み口の契約を検査する。
func RunActivityContract(t *testing.T, newRepos func(t *testing.T) Repos) {
	t.Helper()

	// 2026-10-12（月）〜18（日）の週。
	monday := date(t, 2026, time.October, 12)
	sunday := date(t, 2026, time.October, 18)

	// 壁に開けた穴が Audience の形をしていること。ここが破れると、
	// 同じ保存先にいる全員の記録が見える。
	t.Run("Audience にいる人の記録だけを、期間の両端を含めて返す", func(t *testing.T) {
		r := newRepos(t)
		saveLogs(t, r, userA, logAt("a1", monday, "bench"), logAt("a2", sunday, "squat"),
			logAt("a-before", monday.AddDays(-1), "bench"), logAt("a-after", sunday.AddDays(1), "bench"))
		saveLogs(t, r, userB, logAt("b1", monday.AddDays(2), "squat"))
		saveLogs(t, r, userC, logAt("c1", monday, "bench"))

		got := activity(t, r, audience(t, userA, userA, userB), monday, sunday)
		want := []string{
			userA.String() + " 2026-10-12 ベンチプレス",
			userA.String() + " 2026-10-18 スクワット",
			userB.String() + " 2026-10-14 スクワット",
		}
		if !slices.Equal(got, want) {
			t.Errorf("記録が\n%v\nのはずが\n%v", want, got)
		}
	})

	// 重複は名前で消す（契約の「(人, 日, 種目名) の重複なし」）。消した種目と
	// 同じ名前で作り直した種目を同じ日にやると、ID は2つでも名前は1つ。
	t.Run("同じ日の同じ種目名は何セットあっても1件", func(t *testing.T) {
		r := newRepos(t)
		old := mustExercise(exercise.ExerciseParams{ID: "u-old", Name: "マシンプレス"})
		saveExercise(t, r, userA, old)
		saveLogs(t, r, userA, logAt("1", monday, "bench"), logAt("2", monday, "bench"), logAt("3", monday, "squat"),
			logAt("4", monday, "u-old"))
		saveExercise(t, r, userA, old.Delete())
		saveExercise(t, r, userA, mustExercise(exercise.ExerciseParams{ID: "u-new", Name: "マシンプレス"}))
		saveLogs(t, r, userA, logAt("5", monday, "u-new"))

		got := activity(t, r, audience(t, userA, userA), monday, sunday)
		if len(got) != 3 {
			t.Errorf("%d件。3件（ベンチ・スクワット・マシンプレス）のはず: %v", len(got), got)
		}
	})

	// 名前は持ち主のもの。読む人の一覧で引くと、自作種目は名前が無く、
	// 付け替えた名前は読む人のほうに化ける。
	t.Run("種目名はその人の一覧から引く（付け替え・自作・消した種目も）", func(t *testing.T) {
		r := newRepos(t)
		saveExercise(t, r, userA, mustExercise(exercise.ExerciseParams{ID: "bench", Name: "ベンチ（Aの呼び方）"}))
		saveExercise(t, r, userB, mustExercise(exercise.ExerciseParams{ID: "u-b-custom", Name: "Bの自作"}))
		gone := mustExercise(exercise.ExerciseParams{ID: "u-b-gone", Name: "Bが消した種目"})
		saveExercise(t, r, userB, gone)
		saveLogs(t, r, userA, logAt("a1", monday, "bench"))
		saveLogs(t, r, userB, logAt("b1", monday, "bench"), logAt("b2", monday, "u-b-custom"), logAt("b3", monday, "u-b-gone"))
		saveExercise(t, r, userB, gone.Delete())

		got := activity(t, r, audience(t, userA, userA, userB), monday, sunday)
		want := []string{
			userA.String() + " 2026-10-12 ベンチ（Aの呼び方）",
			userB.String() + " 2026-10-12 Bが消した種目",
			userB.String() + " 2026-10-12 Bの自作",
			userB.String() + " 2026-10-12 ベンチプレス",
		}
		if !slices.Equal(got, want) {
			t.Errorf("記録が\n%v\nのはずが\n%v", want, got)
		}
	})

	// ゼロ値は外から書ける唯一の Audience。それで何かが読めたら穴になる。
	t.Run("ゼロ値の Audience では何も返らない", func(t *testing.T) {
		r := newRepos(t)
		saveLogs(t, r, userA, logAt("a1", monday, "bench"))

		got := activity(t, r, group.Audience{}, monday, sunday)
		if len(got) != 0 {
			t.Errorf("%d件返った: %v", len(got), got)
		}
	})
}

// --- 補助 ---

func mustUserID(s string) account.UserID {
	id, err := account.NewUserID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// date は日付を作る。MustDate はドメインのテストファイルの外では使えない
// （training/architecture_test.go）ので、ここでは t で落とす。
func date(t *testing.T, y int, m time.Month, d int) training.Date {
	t.Helper()
	got, err := training.NewDate(y, m, d)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustExercise(p exercise.ExerciseParams) *exercise.Exercise {
	if p.Stimulus == nil {
		p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 1.0}
	}
	if p.IncrementKg == 0 {
		p.IncrementKg = 2.5
	}
	e, err := exercise.NewExercise(p)
	if err != nil {
		panic(err)
	}
	return e
}

func member(t *testing.T, user account.UserID, name string, joinedAt time.Time) group.Member {
	t.Helper()
	n, err := group.NewDisplayName(name)
	if err != nil {
		t.Fatal(err)
	}
	m, err := group.NewMember(user, n, joinedAt)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func newGroup(t *testing.T, name string, members ...group.Member) *group.Group {
	t.Helper()
	return newGroupAt(t, name, at, members...)
}

func newGroupAt(t *testing.T, name string, createdAt time.Time, members ...group.Member) *group.Group {
	t.Helper()
	id, err := group.NewRandomID()
	if err != nil {
		t.Fatal(err)
	}
	n, err := group.NewName(name)
	if err != nil {
		t.Fatal(err)
	}
	token, err := group.NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	g, err := group.New(group.Params{ID: id, Name: n, Invite: token, Members: members, CreatedAt: createdAt})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func create(t *testing.T, r GroupRepo, g *group.Group) {
	t.Helper()
	if err := r.Create(context.Background(), g); err != nil {
		t.Fatalf("作成に失敗: %v", err)
	}
}

func find(t *testing.T, r GroupRepo, id group.ID) *group.Group {
	t.Helper()
	g, err := r.Find(context.Background(), id)
	if err != nil {
		t.Fatalf("引けない: %v", err)
	}
	return g
}

func memberIDs(g *group.Group) []account.UserID {
	var out []account.UserID
	for _, m := range g.Members() {
		out = append(out, m.UserID())
	}
	return out
}

func assertSameGroup(t *testing.T, got, want *group.Group) {
	t.Helper()
	if got.ID() != want.ID() || got.Name() != want.Name() || got.Invite() != want.Invite() {
		t.Errorf("グループが %s/%s/%s。%s/%s/%s のはず",
			got.ID(), got.Name(), got.Invite(), want.ID(), want.Name(), want.Invite())
	}
	if !got.CreatedAt().Equal(want.CreatedAt()) {
		t.Errorf("作った時刻が %v。%v のはず", got.CreatedAt(), want.CreatedAt())
	}
	gm, wm := got.Members(), want.Members()
	if len(gm) != len(wm) {
		t.Fatalf("メンバーが %d 人。%d 人のはず", len(gm), len(wm))
	}
	for i := range gm {
		if gm[i].UserID() != wm[i].UserID() || gm[i].DisplayName() != wm[i].DisplayName() ||
			!gm[i].JoinedAt().Equal(wm[i].JoinedAt()) {
			t.Errorf("メンバー%d が %s/%s/%v。%s/%s/%v のはず", i,
				gm[i].UserID(), gm[i].DisplayName(), gm[i].JoinedAt(),
				wm[i].UserID(), wm[i].DisplayName(), wm[i].JoinedAt())
		}
	}
}

func logAt(id string, d training.Date, exerciseID string) *setlog.SetLog {
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: id, PerformedOn: d, ExerciseID: exerciseID, WeightKg: 60, Reps: 5, RIR: 2,
	})
	if err != nil {
		panic(err)
	}
	return l
}

func saveLogs(t *testing.T, r Repos, user account.UserID, logs ...*setlog.SetLog) {
	t.Helper()
	// 種目一覧を先に読んでおく。実装によっては最初の読み出しでプリセットを
	// 入れるので、記録だけある人（本番では起きない）を作らない。
	if _, err := r.Exercises.FindAll(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if err := r.SetLogs.Save(context.Background(), user, logs); err != nil {
		t.Fatalf("記録を保存できない: %v", err)
	}
}

func saveExercise(t *testing.T, r Repos, user account.UserID, e *exercise.Exercise) {
	t.Helper()
	if err := r.Exercises.Save(context.Background(), user, e); err != nil {
		t.Fatalf("種目を保存できない: %v", err)
	}
}

// audience は members のグループで viewer が見てよい範囲を返す。
func audience(t *testing.T, viewer account.UserID, members ...account.UserID) group.Audience {
	t.Helper()
	ms := make([]group.Member, 0, len(members))
	for i, u := range members {
		ms = append(ms, member(t, u, "メンバー", at.Add(time.Duration(i)*time.Minute)))
	}
	a, err := newGroup(t, "ジム仲間", ms...).VisibleTo(viewer)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// activity は読んだ記録を「人 日 種目名」の文字列にして並べて返す。
// 並びは契約に無いので、比べる前に揃える。
func activity(t *testing.T, r Repos, a group.Audience, from, to training.Date) []string {
	t.Helper()
	got, err := r.Activity.Activity(context.Background(), a, from, to)
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	out := make([]string, 0, len(got))
	for _, x := range got {
		out = append(out, x.UserID().String()+" "+x.Date().String()+" "+x.ExerciseName())
	}
	slices.Sort(out)
	return out
}
