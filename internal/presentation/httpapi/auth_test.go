package httpapi_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

// Bearer トークン1本の認証（D-068）は、セッションを引くミドルウェアに
// 差し替えた。検査は session_auth_test.go にある。
//
// このファイルに残っているのは、経路そのものの決めごと。

func TestHealthPath_IsNotReservedByCloudRun(t *testing.T) {
	if httpapi.HealthPath == "/healthz" {
		t.Error("/healthz は Cloud Run が横取りするので使えない")
	}
	if httpapi.HealthPath == "" {
		t.Error("ヘルスチェックの経路が空である")
	}
}
