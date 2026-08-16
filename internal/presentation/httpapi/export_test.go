package httpapi

// クエリ文字列の分解は HTTP 経由だと結果が観測しづらい。
// 空の種目IDがドメインへ流れても実害は出ないが、境界で落とすのが正しいので
// 直接テストできるようにする。
var ParseExerciseIDs = parseExerciseIDs
