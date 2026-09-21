package query_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 種目がどの筋区分を刺激するかを返すこと。
//
// 画面が種目の一覧を部位ごとにまとめるのに要る。どれを代表に選ぶかは
// 表示の判断なので画面側で決める。ここは事実をそのまま渡す。
//
// 支配区分（PrimaryRegion）はドメインに作らない。あれは「その種目がどの日に
// 出るか」を決めるためのもので、分割法と一緒に入れると決めてある
// （docs/specs/2026-09-06-training-goals-design.md）。
// infrastructure のリポジトリは使わない。application 層のテストが外側の層に
// 依存すると、architecture_test が依存の向きの違反として弾く。
// 同じパッケージの history_test.go にある stubExercises を使う。
func TestExercises_CarriesStimulus(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	got, err := query.NewExercises(&stubExercises{all: pool}).All(context.Background())
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("種目が1件も返っていない")
	}

	for _, e := range got {
		if len(e.Stimulus) == 0 {
			t.Errorf("%s の刺激分布が空である", e.ID)
		}
		for region, c := range e.Stimulus {
			if !region.Valid() {
				t.Errorf("%s に未知の筋区分: %q", e.ID, region)
			}
			if c <= 0 {
				t.Errorf("%s の %s の寄与度が %v（正であるべき）", e.ID, region, c)
			}
		}
	}
}
