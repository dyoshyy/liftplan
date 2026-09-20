-- 分割の周期をプログラムに持たせる。
--
-- 既存の行は NULL＝分割なし（全身法）。挙動は変わらない。
--
-- jsonb にするのは selected / declared と同じ理由で、要素数が可変で
-- 順序に意味があるため。順序が周期そのものなので、配列の並びを保つ。
ALTER TABLE program ADD COLUMN split_cycle jsonb;
