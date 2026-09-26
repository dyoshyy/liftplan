package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
)

// accountServer はアカウントを入れた状態で、認証を通したルータを返す。
func accountServer(t *testing.T, accounts ...*account.Account) http.Handler {
	t.Helper()
	repo := memory.NewAccountRepository()
	for _, a := range accounts {
		if err := repo.Create(context.Background(), a); err != nil {
			t.Fatalf("アカウントの作成に失敗: %v", err)
		}
	}
	d := dependencies(
		memory.NewExerciseRepository(nil),
		memory.NewSetLogRepository(),
		memory.NewConditionRepository(),
		memory.NewProgramRepository(),
	)
	d.Accounts = query.NewAccounts(repo)
	return authed(t, routesFrom(t, d))
}

func newAccount(t *testing.T, p account.Provider, subject string, u account.UserID, email string) *account.Account {
	t.Helper()
	a, err := account.NewAccount(p, subject, u, account.NewEmail(email))
	if err != nil {
		t.Fatalf("アカウントを作れない: %v", err)
	}
	return a
}

type accountBody struct {
	Accounts []struct {
		Provider string  `json:"provider"`
		Email    *string `json:"email"`
	} `json:"accounts"`
}

func getAccount(t *testing.T, h http.Handler) accountBody {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/account", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	var got accountBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	return got
}

// 設定画面の「アカウント」に、どのアドレスで入っているかを出す。
// 返すのは認証した本人のアカウントだけ。他人の行が混ざると、他人の
// メールアドレスが画面に出る。
func TestGetAccount_ReturnsOnlyTheSignedInUsersLogins(t *testing.T) {
	other := mustTestUserID("22222222-2222-4222-8222-222222222222")
	got := getAccount(t, accountServer(t,
		newAccount(t, account.GitHub(), "1", testUser, "gym@example.com"),
		newAccount(t, account.GitHub(), "2", other, "other@example.com"),
	))

	if len(got.Accounts) != 1 {
		t.Fatalf("%d 件返った。本人の1件のはず: %+v", len(got.Accounts), got.Accounts)
	}
	a := got.Accounts[0]
	if a.Provider != "github" {
		t.Errorf("プロバイダが %q。github のはず", a.Provider)
	}
	if a.Email == nil || *a.Email != "gym@example.com" {
		t.Errorf("アドレスが %v。gym@example.com のはず", a.Email)
	}
}

// アドレスを取っていないアカウントは null で返す。空文字にすると、
// 画面が「空という値のアドレス」と「取っていない」を区別できない。
// 0010 より前に作られたアカウントはこの形。
func TestGetAccount_MissingEmailIsNull(t *testing.T) {
	got := getAccount(t, accountServer(t,
		newAccount(t, account.GitHub(), "1", testUser, ""),
	))

	if len(got.Accounts) != 1 {
		t.Fatalf("%d 件返った。1件のはず", len(got.Accounts))
	}
	if got.Accounts[0].Email != nil {
		t.Errorf("アドレスが %q。null のはず", *got.Accounts[0].Email)
	}
}

// アカウントを持たない利用者（開発用のセッション）は、空の配列で返す。
// null にすると、画面が配列として回したときに落ちる。
func TestGetAccount_NoAccountsIsEmptyArray(t *testing.T) {
	rec := do(t, accountServer(t), http.MethodGet, "/api/account", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	if got, want := rec.Body.String(), "{\"accounts\":[]}\n"; got != want {
		t.Errorf("応答が %q。%q のはず", got, want)
	}
}
