package group

import (
	"context"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// Activity は「その人がその日にその種目をやった」という1件。
//
// 重量・レップ・RIR を持たない。グループで見せるのは日と種目名までと
// 決めた（仕様「何のためか」）ので、運ぶ型そのものにフィールドを置かない。
// 置けば、どこかで詰めてしまう。
type Activity struct {
	userID       account.UserID
	date         training.Date
	exerciseName string
}

// NewActivity は1件を組み立てる。読み口の実装が使う。
func NewActivity(user account.UserID, date training.Date, exerciseName string) Activity {
	return Activity{userID: user, date: date, exerciseName: exerciseName}
}

func (a Activity) UserID() account.UserID { return a.userID }
func (a Activity) Date() training.Date    { return a.date }
func (a Activity) ExerciseName() string   { return a.exerciseName }

// ActivityReader は仲間の記録を読む口。
type ActivityReader interface {
	// Activity は Audience に含まれる人の、from〜to（両端を含む）の記録を
	// (人, 日, 種目名) の重複なしで返す。並びは問わない。
	//
	// 種目名はその人の種目一覧から引く。自作の種目も、消した種目も、
	// その人が付けた名前で返す。
	//
	// 生の []UserID を受け取らないのが要。Audience は Group.VisibleTo
	// からしか作れないので、見てよいかの判断を飛ばして呼べない。
	Activity(ctx context.Context, a Audience, from, to training.Date) ([]Activity, error)
}
