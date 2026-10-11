package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

// グループの経路は2人以上を登場させないと検査できない。
var (
	friendUser    = mustTestUserID("22222222-2222-4222-8222-222222222222")
	friendToken   = "friend-token-0123456789abcdefghijklmnopqrstu"
	strangerUser  = mustTestUserID("33333333-3333-4333-8333-333333333333")
	strangerToken = "stranger-token-0123456789abcdefghijklmnopqr"
)

// groupServer は testUser・friendUser・strangerUser の3人がそれぞれの
// トークンで叩けるサーバー。保存先は3人で共有する（本番と同じ）。
func groupServer(t *testing.T) http.Handler {
	t.Helper()
	store := &sessionStore{byHash: map[string]*account.Session{}}
	for token, user := range map[string]account.UserID{
		sampleToken: testUser, friendToken: friendUser, strangerToken: strangerUser,
	} {
		s := storeWith(t, token, user)
		for k, v := range s.byHash {
			store.byHash[k] = v
		}
	}
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatal(err)
	}
	routes := routesFrom(t, dependencies(memory.NewExerciseRepository(pool), memory.NewSetLogRepository(),
		memory.NewConditionRepository(), memory.NewProgramRepository()))
	return httpapi.RequireSession(store, func() time.Time { return authNow })(routes)
}

// call は token の人として叩き、応答を返す。
func call(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, want int) T {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("ステータスが %d。%d のはず: %s", rec.Code, want, rec.Body)
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON として読めない: %v: %s", err, rec.Body)
	}
	return out
}

type createdGroup struct {
	ID          string `json:"id"`
	InviteToken string `json:"invite_token"`
}

func createGroup(t *testing.T, h http.Handler) createdGroup {
	t.Helper()
	return decode[createdGroup](t, call(t, h, sampleToken, http.MethodPost, "/api/groups",
		map[string]string{"name": "ジム仲間", "display_name": "よし"}), http.StatusCreated)
}

// 作る → リンクを開く → 参加 → 記録 → 仲間の週を見る、を本物の経路で通す。
func TestGroups_InviteJoinAndSeeEachOther(t *testing.T) {
	h := groupServer(t)
	g := createGroup(t, h)

	preview := decode[struct {
		Name          string `json:"name"`
		MemberCount   int    `json:"member_count"`
		AlreadyMember bool   `json:"already_member"`
	}](t, call(t, h, friendToken, http.MethodGet, "/api/invites/"+g.InviteToken, nil), http.StatusOK)
	if preview.Name != "ジム仲間" || preview.MemberCount != 1 || preview.AlreadyMember {
		t.Errorf("リンクの中身が %+v", preview)
	}

	// 二度押しても同じ結果。
	for range 2 {
		joined := decode[struct {
			GroupID string `json:"group_id"`
		}](t, call(t, h, friendToken, http.MethodPost, "/api/invites/"+g.InviteToken+"/join",
			map[string]string{"display_name": "たろう"}), http.StatusOK)
		if joined.GroupID != g.ID {
			t.Errorf("参加したグループが %s。%s のはず", joined.GroupID, g.ID)
		}
	}

	rec := call(t, h, sampleToken, http.MethodPost, "/api/set-logs", map[string]any{"logs": []map[string]any{
		{"id": "s1", "date": "2026-10-12", "exercise_id": "bench", "weight_kg": 87.5, "reps": 5, "rir": 2},
	}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("記録できない: %d %s", rec.Code, rec.Body)
	}

	weekRec := call(t, h, friendToken, http.MethodGet, "/api/groups/"+g.ID+"/week?date=2026-10-14", nil)
	week := decode[struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Members []struct {
			DisplayName string `json:"display_name"`
			IsMe        bool   `json:"is_me"`
			Days        []struct {
				Date      string   `json:"date"`
				Exercises []string `json:"exercises"`
			} `json:"days"`
		} `json:"members"`
	}](t, weekRec, http.StatusOK)

	if week.From != "2026-10-12" || week.To != "2026-10-18" || len(week.Members) != 2 {
		t.Fatalf("週が %+v", week)
	}
	yoshi, taro := week.Members[0], week.Members[1]
	if yoshi.DisplayName != "よし" || yoshi.IsMe || len(yoshi.Days) != 1 ||
		yoshi.Days[0].Date != "2026-10-12" || strings.Join(yoshi.Days[0].Exercises, ",") != "ベンチプレス" {
		t.Errorf("よしの週が %+v", yoshi)
	}
	if taro.DisplayName != "たろう" || !taro.IsMe || taro.Days == nil || len(taro.Days) != 0 {
		t.Errorf("たろうの週が %+v（自分で、記録なし・空の配列のはず）", taro)
	}
	// 見せるのは日と種目名まで。重量が応答に載ったら、型で止めたはずの穴が開いている。
	if body := weekRec.Body.String(); strings.Contains(body, "87.5") || strings.Contains(body, "weight") {
		t.Errorf("重量が応答に出ている: %s", body)
	}
}

// 無い・メンバーでない・形が不正・使えないリンクで、ステータスも本文も同じこと。
// 本文が違うと、隠したいグループの存在が本文から分かる。
func TestGroups_NotFoundLooksTheSame(t *testing.T) {
	h := groupServer(t)
	g := createGroup(t, h)
	old := g.InviteToken
	if rec := call(t, h, sampleToken, http.MethodPost, "/api/groups/"+g.ID+"/invite", nil); rec.Code != http.StatusOK {
		t.Fatalf("リンクを作り直せない: %d %s", rec.Code, rec.Body)
	}

	const missingID = "aaaaaaaa-1111-4111-8111-111111111111"
	cases := []struct {
		name, method, path string
		body               any
	}{
		{"メンバーでない人が週を見る", http.MethodGet, "/api/groups/" + g.ID + "/week?date=2026-10-14", nil},
		{"無いグループの週を見る", http.MethodGet, "/api/groups/" + missingID + "/week?date=2026-10-14", nil},
		{"形の不正な ID の週を見る", http.MethodGet, "/api/groups/not-a-uuid/week?date=2026-10-14", nil},
		{"メンバーでない人がリンクを作り直す", http.MethodPost, "/api/groups/" + g.ID + "/invite", nil},
		{"メンバーでない人が抜ける", http.MethodDelete, "/api/groups/" + g.ID + "/members/me", nil},
		{"作り直す前のリンクを開く", http.MethodGet, "/api/invites/" + old, nil},
		{"作り直す前のリンクで参加する", http.MethodPost, "/api/invites/" + old + "/join", map[string]string{"display_name": "x"}},
	}

	var first string
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := call(t, h, strangerToken, c.method, c.path, c.body)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("ステータスが %d。404 のはず: %s", rec.Code, rec.Body)
			}
			body := rec.Body.String()
			if first == "" {
				first = body
			}
			if body != first {
				t.Errorf("本文が %s。他と同じ %s のはず", body, first)
			}
			if strings.Contains(body, strangerUser.String()) {
				t.Errorf("本文に利用者の ID が出ている: %s", body)
			}
		})
	}
}

func TestGroups_LeaveHidesTheGroup(t *testing.T) {
	h := groupServer(t)
	g := createGroup(t, h)
	call(t, h, friendToken, http.MethodPost, "/api/invites/"+g.InviteToken+"/join", map[string]string{"display_name": "たろう"})

	if rec := call(t, h, friendToken, http.MethodDelete, "/api/groups/"+g.ID+"/members/me", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("抜けられない: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, friendToken, http.MethodGet, "/api/groups/"+g.ID+"/week?date=2026-10-14", nil); rec.Code != http.StatusNotFound {
		t.Errorf("抜けた人が週を見られる: %d", rec.Code)
	}
	list := decode[struct {
		Groups []struct {
			ID string `json:"id"`
		} `json:"groups"`
	}](t, call(t, h, friendToken, http.MethodGet, "/api/groups", nil), http.StatusOK)
	if list.Groups == nil || len(list.Groups) != 0 {
		t.Errorf("抜けた人の一覧に残っている（または null）: %+v", list.Groups)
	}
}
