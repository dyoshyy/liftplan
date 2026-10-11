package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// GroupRepository はグループの Postgres 実装。
//
// 誰がメンバーかを WHERE 句で判断しない。見てよいかは group.Group.VisibleTo が
// 決める（仕様「他人の記録を読む壁をどこに開けるか」）。ここは言われた
// グループを、メンバーごと組み立てて返すだけ。
type GroupRepository struct {
	pool *pgxpool.Pool
}

func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}

// Find はそのグループを返す。
func (r *GroupRepository) Find(ctx context.Context, id group.ID) (*group.Group, error) {
	gs, err := r.load(ctx, `SELECT id::text, name, invite_token, created_at FROM groups WHERE id = $1`, id.String())
	if err != nil {
		return nil, err
	}
	if len(gs) == 0 {
		return nil, fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	return gs[0], nil
}

// FindByInvite はそのトークンのグループを返す。
func (r *GroupRepository) FindByInvite(ctx context.Context, token group.InviteToken) (*group.Group, error) {
	gs, err := r.load(ctx, `SELECT id::text, name, invite_token, created_at FROM groups WHERE invite_token = $1`, token.String())
	if err != nil {
		return nil, err
	}
	if len(gs) == 0 {
		return nil, fmt.Errorf("%w: 招待トークンに当たらない", group.ErrGroupNotFound)
	}
	return gs[0], nil
}

// ListByMember はその人がメンバーのグループを作った順に返す。
func (r *GroupRepository) ListByMember(ctx context.Context, user account.UserID) ([]*group.Group, error) {
	// 同時刻は id で決める。並びが実行計画で変わらないように。
	return r.load(ctx, `
		SELECT g.id::text, g.name, g.invite_token, g.created_at
		  FROM groups g
		  JOIN group_members m ON m.group_id = g.id
		 WHERE m.user_id = $1
		 ORDER BY g.created_at, g.id`, user.String())
}

// groupRow は groups の1行。
type groupRow struct {
	id, name, invite string
	createdAt        time.Time
}

// load は groups を引く SQL を流し、それぞれのメンバーを足して組み立てる。
// 1件も無ければ空のスライスを返す（nil ではない）。
//
// メンバーはグループの一覧を引いたあとにまとめて引く。グループごとに
// 引くと、一覧の件数だけ往復が増える。
func (r *GroupRepository) load(ctx context.Context, query string, arg any) ([]*group.Group, error) {
	rows, err := r.pool.Query(ctx, query, arg)
	if err != nil {
		return nil, wrapUnavailable(err, "グループを読めない")
	}
	heads, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (groupRow, error) {
		var g groupRow
		err := row.Scan(&g.id, &g.name, &g.invite, &g.createdAt)
		return g, err
	})
	if err != nil {
		return nil, wrapUnavailable(err, "グループを読めない")
	}
	if len(heads) == 0 {
		return []*group.Group{}, nil
	}

	ids := make([]string, 0, len(heads))
	for _, h := range heads {
		ids = append(ids, h.id)
	}
	members, err := r.membersOf(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]*group.Group, 0, len(heads))
	for _, h := range heads {
		g, err := rebuild(h, members[h.id])
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

// membersOf はグループごとのメンバーを返す。
func (r *GroupRepository) membersOf(ctx context.Context, ids []string) (map[string][]group.Member, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT group_id::text, user_id::text, display_name, joined_at
		  FROM group_members WHERE group_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, wrapUnavailable(err, "メンバーを読めない")
	}
	defer rows.Close()

	out := make(map[string][]group.Member, len(ids))
	for rows.Next() {
		var (
			groupID, userID, name string
			joinedAt              time.Time
		)
		if err := rows.Scan(&groupID, &userID, &name, &joinedAt); err != nil {
			return nil, wrapUnavailable(err, "メンバーを読めない")
		}
		// 保存されている値も必ずコンストラクタを通す（AccountRepository と同じ）。
		uid, err := account.NewUserID(userID)
		if err != nil {
			return nil, fmt.Errorf("保存されたメンバーが不正: %w", err)
		}
		dn, err := group.NewDisplayName(name)
		if err != nil {
			return nil, fmt.Errorf("保存された表示名が不正: %w", err)
		}
		m, err := group.NewMember(uid, dn, joinedAt)
		if err != nil {
			return nil, fmt.Errorf("保存されたメンバーが不正: %w", err)
		}
		out[groupID] = append(out[groupID], m)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapUnavailable(err, "メンバーを読めない")
	}
	return out, nil
}

// rebuild は行からグループを組み直す。
func rebuild(h groupRow, members []group.Member) (*group.Group, error) {
	id, err := group.ParseID(h.id)
	if err != nil {
		return nil, fmt.Errorf("保存されたグループの ID が不正: %w", err)
	}
	name, err := group.NewName(h.name)
	if err != nil {
		return nil, fmt.Errorf("保存されたグループ名が不正: %w", err)
	}
	token, err := group.ParseInviteToken(h.invite)
	if err != nil {
		return nil, fmt.Errorf("保存された招待トークンが不正: %w", err)
	}
	return group.New(group.Params{ID: id, Name: name, Invite: token, Members: members, CreatedAt: h.createdAt})
}

// Create はグループを最初のメンバーごと保存する。
func (r *GroupRepository) Create(ctx context.Context, g *group.Group) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO groups (id, name, invite_token, created_at) VALUES ($1, $2, $3, $4)`,
			g.ID().String(), g.Name().String(), g.Invite().String(), g.CreatedAt(),
		); err != nil {
			return wrapUnavailable(err, "グループを保存できない")
		}
		for _, m := range g.Members() {
			if err := insertMember(ctx, tx, g.ID(), m); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddMember はメンバーを足す。既にメンバーなら何もしない。
func (r *GroupRepository) AddMember(ctx context.Context, id group.ID, m group.Member) error {
	return insertMember(ctx, r.pool, id, m)
}

// execer は pool と tx の両方が満たす口。
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func insertMember(ctx context.Context, db execer, id group.ID, m group.Member) error {
	_, err := db.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id, display_name, joined_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (group_id, user_id) DO NOTHING`,
		id.String(), m.UserID().String(), m.DisplayName().String(), m.JoinedAt())

	// 外部キーに当たるのは、グループが無い（消えた直後に参加した）とき。
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		return fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	return wrapUnavailable(err, "メンバーを保存できない")
}

// RemoveMember はメンバーを外す。いなければ何もしない。
func (r *GroupRepository) RemoveMember(ctx context.Context, id group.ID, user account.UserID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, id.String(), user.String())
	return wrapUnavailable(err, "メンバーを外せない")
}

// ReplaceInvite は招待トークンを差し替える。
func (r *GroupRepository) ReplaceInvite(ctx context.Context, id group.ID, token group.InviteToken) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE groups SET invite_token = $2 WHERE id = $1`, id.String(), token.String())
	if err != nil {
		return wrapUnavailable(err, "招待トークンを差し替えられない")
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", group.ErrGroupNotFound, id)
	}
	return nil
}

// DeleteIfEmpty はメンバーがいなければグループを消す。
func (r *GroupRepository) DeleteIfEmpty(ctx context.Context, id group.ID) error {
	//
	// 空かどうかの確認と削除を1文にする。分けると、その間に入った人ごと
	// ON DELETE CASCADE で消える。
	_, err := r.pool.Exec(ctx, `
		DELETE FROM groups g
		 WHERE g.id = $1
		   AND NOT EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = g.id)`, id.String())
	return wrapUnavailable(err, "グループを消せない")
}

// GroupActivityReader は仲間の記録の Postgres 実装。
type GroupActivityReader struct {
	pool *pgxpool.Pool
}

func NewGroupActivityReader(pool *pgxpool.Pool) *GroupActivityReader {
	return &GroupActivityReader{pool: pool}
}

// Activity は Audience の人の from〜to の記録を (人, 日, 種目名) で返す。
//
// 重量・レップ・RIR は SELECT しない。運ぶ型（group.Activity）にも
// フィールドが無いが、そもそも DB から持ち出さない。
//
// 種目名は記録の持ち主の user_exercises から引く。消した種目も行は
// 残っている（論理削除）ので名前が出る。名前の行が無い記録（本番では
// 起きない。記録の保存が種目の一覧を先に読む）も、日が抜けないように
// 空の名前で返す。
func (r *GroupActivityReader) Activity(
	ctx context.Context, a group.Audience, from, to training.Date,
) ([]group.Activity, error) {
	users := a.UserIDs()
	if len(users) == 0 {
		return []group.Activity{}, nil
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.String())
	}

	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT l.user_id::text, l.performed_on, coalesce(e.name, '')
		  FROM set_logs l
		  LEFT JOIN user_exercises e ON e.user_id = l.user_id AND e.id = l.exercise_id
		 WHERE l.user_id = ANY($1::uuid[])
		   AND l.performed_on BETWEEN $2 AND $3`,
		ids, toTime(from), toTime(to))
	if err != nil {
		return nil, wrapUnavailable(err, "仲間の記録を読めない")
	}
	defer rows.Close()

	out := []group.Activity{}
	for rows.Next() {
		var (
			userID, name string
			performedOn  time.Time
		)
		if err := rows.Scan(&userID, &performedOn, &name); err != nil {
			return nil, wrapUnavailable(err, "仲間の記録を読めない")
		}
		uid, err := account.NewUserID(userID)
		if err != nil {
			return nil, fmt.Errorf("保存された記録の持ち主が不正: %w", err)
		}
		d, err := training.FromTime(performedOn, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("保存された記録の日付が不正: %w", err)
		}
		out = append(out, group.NewActivity(uid, d, name))
	}
	if err := rows.Err(); err != nil {
		return nil, wrapUnavailable(err, "仲間の記録を読めない")
	}
	return out, nil
}
