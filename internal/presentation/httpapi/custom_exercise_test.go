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
// 刻みを差し替え、GET に反映されることを見る。
func TestExercises_AddEdit(t *testing.T) {
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
}

// 種目は消せる（論理削除）。プリセット由来も自分で足した種目も同じ扱い。
//
// 2026-10-01（#238）に一度「消せない（DELETE は 405）」にしたが、本人の依頼で
// 戻した。そのときのこのテストは、DELETE が届かず一覧にも残ることを見ていた。
// いまは消した種目も GET に deleted: true で残る（履歴の名前のため）ことを見る。
// 「使う・使わない」はそのまま残っている。
func TestExercises_CanBeDeleted(t *testing.T) {
	srv := newServer(t, true)

	if rec := do(t, srv, http.MethodDelete, "/api/exercises/side_raise", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE が %d。204 のはず: %s", rec.Code, rec.Body.String())
	}

	rec := do(t, srv, http.MethodGet, "/api/exercises", "")
	var list struct {
		Exercises []exerciseWire `json:"exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	deleted := false
	for _, e := range list.Exercises {
		if e.ID == "side_raise" {
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
// ステータスだけだと、DELETE・PUT のルート登録を消しても「無い ID」が誤って
// 緑のままになる（実際にミューテーションで確認済み）。
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
		// wantAbsent は本文に含まれてはいけない部分文字列。空なら見ない。
		// 「名前で伝わっている」ことを、raw ID が出ていないことの側からも確かめる。
		wantAbsent string
		// setup は c の本番のリクエストの前に、同じ mux へ追加のリクエストを
		// 行う。nil なら newServer(t, true) の既定状態のまま本番のリクエストを送る。
		setup func(t *testing.T, mux http.Handler)
	}{
		{
			name: "寄与1.0が無い", method: http.MethodPost, path: "/api/exercises",
			body:     `{"name":"x","stimulus":{"LAT":0.5},"increment_kg":2.5}`,
			want:     400,
			wantCode: "INVALID_INPUT", wantMsg: "寄与1.0",
		},
		{
			// 名前が読めることとフレーズが1回しか出ないことは、下の
			// DUPLICATE_NAME 分岐と wantMsg の両方で見る。
			name: "共通と同名", method: http.MethodPost, path: "/api/exercises",
			body:     `{"name":"サイドレイズ","stimulus":{"SIDE_DELT":1},"increment_kg":2.5}`,
			want:     409,
			wantCode: "DUPLICATE_NAME", wantMsg: "サイドレイズ",
		},
		{
			// PUT は編集中なので「足せない」ではなく、足す・直すの両方に
			// 合う言い方（apperror.ErrDuplicateName「種目を保存できない」）
			// になっていること。
			name: "PUT 共通と同名", method: http.MethodPut, path: "/api/exercises/side_raise",
			body:     `{"name":"ベンチプレス","stimulus":{"SIDE_DELT":1},"increment_kg":2.5}`,
			want:     409,
			wantCode: "DUPLICATE_NAME", wantMsg: "保存できない",
		},
		{
			name: "PUT 無い ID", method: http.MethodPut, path: "/api/exercises/nonexistent",
			body:     `{"name":"x","stimulus":{"LAT":1},"increment_kg":2.5}`,
			want:     404,
			wantCode: "EXERCISE_NOT_FOUND",
		},
		{
			name: "存在しない種目を消す", method: http.MethodDelete, path: "/api/exercises/nonexistent",
			want:     404,
			wantCode: "EXERCISE_NOT_FOUND",
		},
		{
			// newServer(t, true) の伸ばしたい種目は bench・squat・deadlift。
			name: "伸ばしたい種目を消す", method: http.MethodDelete, path: "/api/exercises/bench",
			want:     409,
			wantCode: "STILL_DECLARED",
		},
		{
			// プリセット由来かどうかで扱いを変えない。伸ばしたい種目に
			// 入っていないプリセットは消せる。
			name: "プリセットを消す", method: http.MethodDelete, path: "/api/exercises/side_raise",
			want: 204,
		},
		{
			// 分割を「胸のみ」に絞ったうえで、宣言種目 bench（ChestMid 主働）の
			// 効き方を胸を含まない形へ直すと、bench がどの分割日にも出られなく
			// なる。原因は「いま直した効き方」なので、メッセージは raw ID の
			// "bench" ではなく名前「ベンチプレス」で伝わること
			// （verifyDeclaredHaveADay ではなく editLosesADayError を経由する）。
			name:   "PUT で宣言種目が分割日を失う",
			method: http.MethodPut, path: "/api/exercises/bench",
			body:       `{"name":"ベンチプレス","stimulus":{"QUAD":1},"increment_kg":2.5}`,
			want:       400,
			wantCode:   "INVALID_INPUT",
			wantMsg:    "ベンチプレス",
			wantAbsent: `"bench"`,
			setup: func(t *testing.T, mux http.Handler) {
				t.Helper()
				// squat・deadlift を宣言から外す。この分割は胸しか含まないので、
				// 外さないとこの2つも同時に分割日を失い、この setup 自体が
				// 400 で失敗してしまう。
				if rec := do(t, mux, http.MethodPut, "/api/program/declared",
					`{"declared_exercises":["bench"]}`); rec.Code != http.StatusNoContent {
					t.Fatalf("宣言の変更に失敗: %d %s", rec.Code, rec.Body.String())
				}
				if rec := do(t, mux, http.MethodPut, "/api/program/split",
					`{"splits":[{"name":"胸のみ","regions":["CHEST_MID"]}]}`); rec.Code != http.StatusNoContent {
					t.Fatalf("分割の設定に失敗: %d %s", rec.Code, rec.Body.String())
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mux := newServer(t, true)
			if c.setup != nil {
				c.setup(t, mux)
			}
			rec := do(t, mux, c.method, c.path, c.body)
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
			if c.wantAbsent != "" && strings.Contains(body.Error, c.wantAbsent) {
				t.Errorf("エラーメッセージに出てはいけない %q がある: %q", c.wantAbsent, body.Error)
			}
			// DUPLICATE_NAME は apperror.ErrDuplicateName とドメインの
			// exercise.ErrDuplicateExerciseName を ": " で連結して返す。
			// 両者の文言が同じだと「同じ名前の種目がある: 同じ名前の種目がある: X」
			// と重複するので、フレーズが1回しか出ないことを見る（名前が
			// 読めることは各ケースの wantMsg で見る）。
			if c.wantCode == "DUPLICATE_NAME" {
				const phrase = "同じ名前の種目がある"
				if strings.Count(body.Error, phrase) != 1 {
					t.Errorf("エラーメッセージでフレーズが重複している: %q", body.Error)
				}
			}
		})
	}
}
