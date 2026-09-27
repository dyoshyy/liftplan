package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// exerciseWire は POST/PUT/GET が運ぶ種目の形。3つの経路で共通に使う。
type exerciseWire struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Stimulus    map[string]float64 `json:"stimulus"`
	IncrementKg float64            `json:"increment_kg"`
	Deleted     bool               `json:"deleted"`
}

// 足す→直す→消すの一連の流れが GET に正しく反映されること。
//
// プリセット由来かどうかで扱いを変えない（設計書「custom は持たない」）ので、
// ここでは自分が足した種目だけを主役にする。PUT が同じ本文で名前・効き方・
// 刻みを差し替え、DELETE の後も GET には deleted: true で残ることを見る。
func TestExercises_AddEditDelete(t *testing.T) {
	srv := newServer(t, true)

	rec := do(t, srv, http.MethodPost, "/api/exercises",
		`{"name":"アイソラテラル・ロー","stimulus":{"TRAP_MID":1,"LAT":0.5,"BICEPS":0.5},"increment_kg":2.5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST が %d: %s", rec.Code, rec.Body.String())
	}
	var created exerciseWire
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if created.Stimulus["TRAP_MID"] != 1.0 || created.Stimulus["LAT"] != 0.5 {
		t.Errorf("作った種目が違う: %+v", created)
	}

	rec = do(t, srv, http.MethodPut, "/api/exercises/"+created.ID,
		`{"name":"シーテッドロー","stimulus":{"LAT":1},"increment_kg":5}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT が %d: %s", rec.Code, rec.Body.String())
	}
	var edited exerciseWire
	if err := json.Unmarshal(rec.Body.Bytes(), &edited); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if edited.Name != "シーテッドロー" || edited.IncrementKg != 5 ||
		edited.Stimulus["LAT"] != 1.0 || len(edited.Stimulus) != 1 {
		t.Errorf("直した種目が違う: %+v", edited)
	}

	rec = do(t, srv, http.MethodGet, "/api/exercises", "")
	var list struct {
		Exercises []exerciseWire `json:"exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	var afterEdit *exerciseWire
	for i, e := range list.Exercises {
		if e.ID == created.ID {
			afterEdit = &list.Exercises[i]
		}
	}
	if afterEdit == nil {
		t.Fatal("直した種目が一覧に無い")
	}
	if afterEdit.Name != "シーテッドロー" || afterEdit.Stimulus["LAT"] != 1.0 || afterEdit.Deleted {
		t.Errorf("GET に PUT の結果が反映されていない: %+v", afterEdit)
	}

	if rec := do(t, srv, http.MethodDelete, "/api/exercises/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE が %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, srv, http.MethodGet, "/api/exercises", "")
	list.Exercises = nil
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	deleted := false
	for _, e := range list.Exercises {
		if e.ID == created.ID {
			deleted = e.Deleted
		}
	}
	if !deleted {
		t.Error("消した種目が deleted: true で一覧に残っていない")
	}
}

// エラーの分類が正しいこと。
//
// code まで見るのは、ルートが無いだけの 404（ServeMux の素の
// "404 page not found"）と、EXERCISE_NOT_FOUND を区別するため。
// ステータスだけだと、DELETE のルート登録を消しても「共通の種目を消す」が
// 誤って緑のままになる（実際にミューテーションで確認済み）。
func TestExercises_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		want                     int
		wantCode                 string
		// wantMsg は本文に含まれるべき部分文字列。空なら見ない。
		//
		// 「寄与1.0が無い」を単に 400/INVALID_INPUT だけで見ると、本文が
		// 何らかの理由で解釈できないだけでも同じ判定になり、寄与1.0の規則を
		// 検査していないテストになる（decodeJSON の失敗も同じ code を返す）。
		// hasFullContribution が実際に働いたことを、そのエラー文言で見る。
		wantMsg string
	}{
		{
			"寄与1.0が無い", http.MethodPost, "/api/exercises",
			`{"name":"x","stimulus":{"LAT":0.5},"increment_kg":2.5}`,
			400, "INVALID_INPUT", "寄与1.0",
		},
		{
			"共通と同名", http.MethodPost, "/api/exercises",
			`{"name":"サイドレイズ","stimulus":{"SIDE_DELT":1},"increment_kg":2.5}`,
			409, "DUPLICATE_NAME", "",
		},
		{
			"PUT 無い ID", http.MethodPut, "/api/exercises/nonexistent",
			`{"name":"x","stimulus":{"LAT":1},"increment_kg":2.5}`,
			404, "EXERCISE_NOT_FOUND", "",
		},
		{
			"存在しない種目を消す", http.MethodDelete, "/api/exercises/nonexistent", "",
			404, "EXERCISE_NOT_FOUND", "",
		},
		{
			// newServer(t, true) の伸ばしたい種目は bench・squat・deadlift。
			"伸ばしたい種目を消す", http.MethodDelete, "/api/exercises/bench", "",
			409, "STILL_DECLARED", "",
		},
		{
			// プリセット由来かどうかで扱いを変えない。伸ばしたい種目に
			// 入っていないプリセットは消せる。
			"プリセットを消す", http.MethodDelete, "/api/exercises/side_raise", "",
			204, "", "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, true), c.method, c.path, c.body)
			if rec.Code != c.want {
				t.Errorf("%d（期待 %d）: %s", rec.Code, c.want, rec.Body.String())
			}
			if c.want == http.StatusNoContent {
				return
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
			if c.wantMsg != "" && !strings.Contains(body.Error, c.wantMsg) {
				t.Errorf("エラーメッセージに %q が無い: %q", c.wantMsg, body.Error)
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
