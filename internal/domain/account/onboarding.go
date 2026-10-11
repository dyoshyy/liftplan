package account

import (
	"context"
	"time"
)

// OnboardingReader は初回の案内を済ませたかを読む。
//
// 行が無いことを「まだ」と読む。初回ログインで行を作らずに済み、既存の
// 利用者はマイグレーション（0020）で済んだ行を持つ。
type OnboardingReader interface {
	OnboardingCompleted(ctx context.Context, user UserID) (bool, error)
}

// OnboardingWriter は初回の案内を済ませたことを記録する。
//
// 何度呼んでも同じ結果になる。画面は送信に失敗すると最初から送り直すので、
// 2回目を拒むと、済んだのに「済ませられない」と出る。
type OnboardingWriter interface {
	CompleteOnboarding(ctx context.Context, user UserID, at time.Time) error
}
