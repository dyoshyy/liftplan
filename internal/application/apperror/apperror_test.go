package apperror_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
)

// ラップされても errors.As で取り出せること。
//
// ユースケースは fmt.Errorf("%w: 頻度: %w", apperror.ErrInvalidInput, err)
// のように文脈を足して返す。取り出せないと presentation が全部 500 にする。
func TestError_SurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("設定の保存に失敗: %w: 頻度: %w",
		apperror.ErrInvalidInput, errors.New("週の頻度が範囲外"))

	var coded *apperror.Error
	if !errors.As(err, &coded) {
		t.Fatal("ラップすると取り出せない")
	}
	if coded.HTTPStatus() != http.StatusBadRequest {
		t.Errorf("ステータスが %d。400 のはず", coded.HTTPStatus())
	}
	if coded.Code() != "INVALID_INPUT" {
		t.Errorf("コードが %q", coded.Code())
	}
}

// ドメインのセンチネルが連鎖に残ること。
//
// ユースケースの利用者は errors.Is でドメインのセンチネルを見ている。
// 分類を足したせいでそれが切れると、判断の根拠が消える。
func TestError_KeepsTheDomainSentinel(t *testing.T) {
	domain := errors.New("プログラムが未設定である")
	err := fmt.Errorf("%w: %w", apperror.ErrNotConfigured, domain)

	if !errors.Is(err, domain) {
		t.Error("ドメインのセンチネルが連鎖から消えている")
	}
	if !errors.Is(err, apperror.ErrNotConfigured) {
		t.Error("分類が連鎖から消えている")
	}
}

// 別の分類とは一致しないこと。
func TestError_IsDistinguishesCodes(t *testing.T) {
	if errors.Is(apperror.ErrInvalidInput, apperror.ErrConflict) {
		t.Error("違うコードが一致している")
	}
	if !errors.Is(apperror.ErrConflict, apperror.ErrConflict) {
		t.Error("同じコードが一致しない")
	}
}

// 分類ごとにステータスが違うこと。同じ値が並ぶと分けた意味が無い。
func TestError_StatusesAreDistinct(t *testing.T) {
	cases := map[*apperror.Error]int{
		apperror.ErrInvalidInput:     http.StatusBadRequest,
		apperror.ErrNotConfigured:    http.StatusConflict,
		apperror.ErrConflict:         http.StatusConflict,
		apperror.ErrDuplicateName:    http.StatusConflict,
		apperror.ErrExerciseNotFound: http.StatusNotFound,
		apperror.ErrUnavailable:      http.StatusServiceUnavailable,
		apperror.ErrTooLarge:         http.StatusRequestEntityTooLarge,
		apperror.ErrTimeout:          http.StatusGatewayTimeout,
		apperror.ErrInternal:         http.StatusInternalServerError,
	}
	codes := map[string]bool{}
	for e, want := range cases {
		if e.HTTPStatus() != want {
			t.Errorf("%s のステータスが %d。%d のはず", e.Code(), e.HTTPStatus(), want)
		}
		if codes[e.Code()] {
			t.Errorf("コード %q が重複している", e.Code())
		}
		codes[e.Code()] = true
	}
}
