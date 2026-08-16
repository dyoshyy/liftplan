package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// wrapUnavailable は「保存先に到達できない」類の失敗を翻訳する。
//
// 接続断・接続数の上限・シャットダウン中は、いずれも後で送り直せば通る。
// 500 のまま返すとクライアントが諦めるので、区別できる形にする。
func wrapUnavailable(err error, what string) error {
	if err == nil {
		return nil
	}
	if isUnavailable(err) {
		return fmt.Errorf("%s: %w: %w", what, training.ErrRepositoryUnavailable, err)
	}
	return fmt.Errorf("%s: %w", what, err)
}

func isUnavailable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgerrcode.TooManyConnections,
			pgerrcode.AdminShutdown,
			pgerrcode.CrashShutdown,
			pgerrcode.CannotConnectNow:
			return true
		}
		// クラス 08 は接続例外。
		return len(pgErr.Code) >= 2 && pgErr.Code[:2] == "08"
	}
	// 接続が張れない・切れた場合は PgError にならない。
	var connErr *pgconn.ConnectError
	return errors.As(err, &connErr)
}
