package query

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/group"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// Groups はグループを読む経路（一覧・今週・招待リンクの中身）。
type Groups struct {
	groups   group.Reader
	activity group.ActivityReader
}

func NewGroups(groups group.Reader, activity group.ActivityReader) *Groups {
	return &Groups{groups: groups, activity: activity}
}

// GroupSummary は一覧の1件。
type GroupSummary struct {
	ID          group.ID
	Name        string
	MemberCount int
	// MyName はこのグループでの自分の表示名。
	MyName string
}

// Mine は自分がメンバーのグループを作った順に返す。
func (q *Groups) Mine(ctx context.Context, user account.UserID) (_ []GroupSummary, err error) {
	defer func() { err = apperror.Classify(err) }()

	gs, err := q.groups.ListByMember(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("グループの一覧を読めない: %w", err)
	}
	out := make([]GroupSummary, 0, len(gs))
	for _, g := range gs {
		s := GroupSummary{ID: g.ID(), Name: g.Name().String(), MemberCount: len(g.Members())}
		for _, m := range g.Members() {
			if m.UserID() == user {
				s.MyName = m.DisplayName().String()
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// GroupWeek はグループの1週間。
type GroupWeek struct {
	ID     group.ID
	Name   string
	Invite group.InviteToken
	From   training.Date // 月曜
	To     training.Date // 日曜
	// Members は参加順。
	Members []MemberWeek
}

// MemberWeek はメンバー1人の1週間。
type MemberWeek struct {
	DisplayName string
	IsMe        bool
	// Days はやった日だけを日付順に持つ。
	Days []MemberDay
}

// MemberDay はその日にやった種目。
type MemberDay struct {
	Date training.Date
	// Exercises は種目名の昇順。名前を引けなかった種目は空文字で入る。
	Exercises []string
}

// Week は date を含む週（月〜日）の、メンバーごとの記録を返す。
//
// メンバーでなければ ErrGroupNotFound（存在を漏らさない）。
func (q *Groups) Week(ctx context.Context, user account.UserID, rawID string, date training.Date) (_ GroupWeek, err error) {
	defer func() { err = apperror.Classify(err) }()

	id, err := group.ParseID(rawID)
	if err != nil {
		return GroupWeek{}, err
	}
	g, err := q.groups.Find(ctx, id)
	if err != nil {
		return GroupWeek{}, err
	}
	audience, err := g.VisibleTo(user)
	if err != nil {
		return GroupWeek{}, err
	}

	from, to := weekOf(date)
	acts, err := q.activity.Activity(ctx, audience, from, to)
	if err != nil {
		return GroupWeek{}, fmt.Errorf("仲間の記録を読めない: %w", err)
	}

	byUser := map[account.UserID]map[training.Date][]string{}
	for _, a := range acts {
		days := byUser[a.UserID()]
		if days == nil {
			days = map[training.Date][]string{}
			byUser[a.UserID()] = days
		}
		days[a.Date()] = append(days[a.Date()], a.ExerciseName())
	}

	out := GroupWeek{ID: g.ID(), Name: g.Name().String(), Invite: g.Invite(), From: from, To: to}
	for _, m := range audience.Members() {
		mw := MemberWeek{DisplayName: m.DisplayName().String(), IsMe: m.UserID() == user, Days: []MemberDay{}}
		for d := from; !d.After(to); d = d.AddDays(1) {
			names, ok := byUser[m.UserID()][d]
			if !ok {
				continue
			}
			slices.SortFunc(names, strings.Compare)
			mw.Days = append(mw.Days, MemberDay{Date: d, Exercises: names})
		}
		out.Members = append(out.Members, mw)
	}
	return out, nil
}

// weekOf は date を含む週の月曜と日曜を返す。
func weekOf(date training.Date) (training.Date, training.Date) {
	// time.Weekday は日曜が0。月曜からの日数に直す。
	offset := (int(date.Weekday()) + 6) % 7
	monday := date.AddDays(-offset)
	return monday, monday.AddDays(6)
}

// InvitePreview は招待リンクを開いたときに見せる中身。
type InvitePreview struct {
	ID            group.ID
	Name          string
	MemberCount   int
	AlreadyMember bool
}

// Invite は招待トークンのグループを返す。使えないトークンは ErrGroupNotFound。
//
// メンバーでなくても読める（リンクを持っていることが参加の資格）。
// 返すのは名前と人数だけで、メンバーの表示名は返さない。参加する前に
// 誰がいるかまで見せる理由が無い。
func (q *Groups) Invite(ctx context.Context, user account.UserID, rawToken string) (_ InvitePreview, err error) {
	defer func() { err = apperror.Classify(err) }()

	token, err := group.ParseInviteToken(rawToken)
	if err != nil {
		return InvitePreview{}, err
	}
	g, err := q.groups.FindByInvite(ctx, token)
	if err != nil {
		return InvitePreview{}, err
	}
	return InvitePreview{
		ID: g.ID(), Name: g.Name().String(), MemberCount: len(g.Members()), AlreadyMember: g.IsMember(user),
	}, nil
}
