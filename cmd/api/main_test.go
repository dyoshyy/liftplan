package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

func TestBuildHandler_ServesSession(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuildHandler_Healthz(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ヘルスチェックが失敗: %d", rec.Code)
	}
}

// 初期プログラムでセッションが導出できること。
// 起動直後に PUT /api/program を叩かないと何も使えない状態を避ける。
func TestBuildHandler_WorksOutOfTheBox(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		Main        []json.RawMessage `json:"main"`
		Accessories []json.RawMessage `json:"accessories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.Main) != 3 {
		t.Errorf("メイン種目が3つでない: %d", len(got.Main))
	}
	if len(got.Accessories) == 0 {
		t.Error("補助種目が1つも出ていない")
	}
}

// 実績を記録してから計画を取り直すと重量が確定すること。
// 層をまたいだ往復がここで初めて通る。
func TestBuildHandler_RecordThenPlan(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}

	post := func(t *testing.T, path, body string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s が失敗: %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	var logs []string
	for i := range 3 {
		for s := range 3 {
			logs = append(logs, fmt.Sprintf(
				`{"id":"e%d-%d","date":"2026-07-%02d","exercise_id":"bench",`+
					`"weight_kg":85,"reps":8,"rir":2}`, i, s, 6+i*7))
		}
	}
	post(t, "/api/set-logs", `{"logs":[`+strings.Join(logs, ",")+`]}`)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}

	var got struct {
		Main []struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
		} `json:"main"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	for _, m := range got.Main {
		if m.ExerciseID != "bench" {
			continue
		}
		if m.WeightKg == nil {
			t.Fatal("記録したのに重量が未確定")
		}
		if *m.WeightKg <= 0 {
			t.Errorf("重量が不正: %v", *m.WeightKg)
		}
		return
	}
	t.Fatal("bench がメインに無い")
}

// PORT が空なら既定値を使うこと。
func TestPort(t *testing.T) {
	t.Setenv("PORT", "")
	if got := port(); got != "8080" {
		t.Errorf("既定のポートが誤り: %s", got)
	}
	t.Setenv("PORT", "9999")
	if got := port(); got != "9999" {
		t.Errorf("PORT が反映されない: %s", got)
	}
}

// 初期プログラムにバリエーションを含めないこと。
// 含めると、バリエーションがメイン扱いで独立したスロットを持つ。
func TestDefaultProgram_ExcludesVariations(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	program, err := defaultProgram(pool)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}

	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			continue
		}
		if program.Includes(e.ID()) {
			t.Errorf("バリエーション %s が選択に含まれている", e.ID())
		}
	}
}
