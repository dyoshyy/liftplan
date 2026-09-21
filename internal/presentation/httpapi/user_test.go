package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 認証ミドルウェアを通っていない要求は、処理せずに 500 で落ちる。
//
// 守っているのは「認証を通っていないのに記録が書き換わらない」こと。
// 利用者が取り出せないときに既定ユーザーで続行する実装にすると、
// ミドルウェアを外した配線ミスや、経路の付け忘れが、そのまま
// 「誰のものでもない要求が既定ユーザーの記録を読み書きする」になる。
// しかも応答は 204 なので、誰も気づけない。
//
// 401 ではなく 500 を期待するのは、原因が送り主ではなくこちら側の
// 配線にあるため。401 を返すとクライアントは認証をやり直すが、
// 何度やっても通らない。
func TestRoutes_RefusesRequestWithoutUser(t *testing.T) {
	routes := buildRoutes(t, true) // ミドルウェアを被せない
	server := authed(t, routes)    // 同じリポジトリを認証経由で覗く用

	payload := `{"logs":[` +
		`{"id":"01J-A","date":"2026-08-17","exercise_id":"bench",` +
		`"weight_kg":85,"reps":9,"rir":2}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("利用者が無い要求が %d で通った。500 のはず: body=%s",
			rec.Code, rec.Body.String())
	}

	// ステータスだけ見ても足りない。落としたつもりで保存されていたら、
	// 守りたかったものが守れていない。
	got := httptest.NewRecorder()
	server.ServeHTTP(got, httptest.NewRequest(
		http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-31", nil))

	// 200 を先に確かめる。エラー応答でも Days は空で読めてしまい、
	// 下の走査が1度も回らないまま緑になる。
	if got.Code != http.StatusOK {
		t.Fatalf("記録の取得に失敗した: %d body=%s", got.Code, got.Body.String())
	}

	var body struct {
		Days []struct {
			TotalSets int `json:"total_sets"`
		} `json:"days"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	for _, d := range body.Days {
		if d.TotalSets != 0 {
			t.Fatalf("利用者が無い要求で記録が保存された: %s", got.Body.String())
		}
	}
}
