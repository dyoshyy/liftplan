package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
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
//
// code まで見るのは、ルートが無いだけの 404（ServeMux の素の
// "404 page not found"）と、EXERCISE_NOT_FOUND を区別するため。
// ステータスだけだと、DELETE のルート登録を消しても「共通の種目を消す」が
// 誤って緑のままになる（実際にミューテーションで確認済み）。
func TestCustomExercises_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		want                     int
		wantCode                 string
	}{
		{"主なし", http.MethodPost, "/api/exercises", `{"name":"x","primary":[],"secondary":[],"increment_kg":2.5}`, 400, "INVALID_INPUT"},
		{"共通と同名", http.MethodPost, "/api/exercises", `{"name":"サイドレイズ","primary":["SIDE_DELT"],"secondary":[],"increment_kg":2.5}`, 409, "DUPLICATE_NAME"},
		{"共通の種目を消す", http.MethodDelete, "/api/exercises/side_raise", "", 404, "EXERCISE_NOT_FOUND"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, true), c.method, c.path, c.body)
			if rec.Code != c.want {
				t.Errorf("%d（期待 %d）: %s", rec.Code, c.want, rec.Body.String())
			}
			var body struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("応答を解釈できない: %v", err)
			}
			if body.Code != c.wantCode {
				t.Errorf("コードが %q。%q のはず", body.Code, c.wantCode)
			}
			// DUPLICATE_NAME は apperror.ErrDuplicateName とドメインの
			// exercise.ErrDuplicateExerciseName を ": " で連結して返す。
			// 両者の文言が同じだと「同じ名前の種目がある: 同じ名前の種目がある: サイドレイズ」
			// と重複するので、種目名が読めることと、フレーズが1回しか
			// 出ないことを見る。
			if c.wantCode == "DUPLICATE_NAME" {
				const phrase = "同じ名前の種目がある"
				if !strings.Contains(body.Error, "サイドレイズ") {
					t.Errorf("エラーメッセージに種目名が無い: %q", body.Error)
				}
				if strings.Count(body.Error, phrase) != 1 {
					t.Errorf("エラーメッセージでフレーズが重複している: %q", body.Error)
				}
			}
		})
	}
}
