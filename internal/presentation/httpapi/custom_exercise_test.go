package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// 自分の種目を足して、一覧に出て、消せること。
//
// 消した種目は一覧から消えない（Deleted: true で残る）。履歴の実績が
// 種目名を引けるようにするため。
func TestCustomExercises_AddListDelete(t *testing.T) {
	srv := newServer(t, true)

	rec := do(t, srv, http.MethodPost, "/api/exercises",
		`{"name":"アイソラテラル・ロー","primary":["TRAP_MID"],"secondary":["LAT","BICEPS"],"increment_kg":2.5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST が %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID       string             `json:"id"`
		Custom   bool               `json:"custom"`
		Stimulus map[string]float64 `json:"stimulus"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !created.Custom || created.Stimulus["TRAP_MID"] != 1.0 || created.Stimulus["LAT"] != 0.5 {
		t.Errorf("作った種目が違う: %+v", created)
	}

	if rec := do(t, srv, http.MethodDelete, "/api/exercises/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE が %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, srv, http.MethodGet, "/api/exercises", "")
	var list struct {
		Exercises []struct {
			ID      string `json:"id"`
			Deleted bool   `json:"deleted"`
		} `json:"exercises"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, e := range list.Exercises {
		if e.ID == created.ID {
			found = e.Deleted
		}
	}
	if !found {
		t.Error("消した種目が deleted: true で一覧に残っていない")
	}
}

// エラーの分類が正しいこと。
func TestCustomExercises_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"主なし", http.MethodPost, "/api/exercises", `{"name":"x","primary":[],"secondary":[],"increment_kg":2.5}`, 400},
		{"共通と同名", http.MethodPost, "/api/exercises", `{"name":"サイドレイズ","primary":["SIDE_DELT"],"secondary":[],"increment_kg":2.5}`, 409},
		{"共通の種目を消す", http.MethodDelete, "/api/exercises/side_raise", "", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, true), c.method, c.path, c.body)
			if rec.Code != c.want {
				t.Errorf("%d（期待 %d）: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}
