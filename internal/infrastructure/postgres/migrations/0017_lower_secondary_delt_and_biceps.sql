-- プレスの側部三角筋と、引く種目の二頭筋の副次寄与を 0.3 に下げる。
--
-- 0.5 だと、肩の日にショルダープレスが2本入るだけで側部三角筋が週目標の
-- 大半を埋め、サイドレイズが選ばれない。二頭筋も同じで、ロー・プルダウンの
-- 副次だけで埋まってカールが出ない。副次の寄与は「狙わなくても埋まる量」を
-- 決めるので、厚く置くとその区分を主働にする種目の出番を奪う
-- （seed/weekly_target.go の stimulusPerSet のコメント）。
--
-- 種目は利用者ごとにコピーされている（0014）ので、シードを直すだけでは
-- 既存の利用者に届かない。シードの旧値のままの行だけを書き換える。本人が
-- 効き方を直した行（値が旧値と違う）は触らない。消した行も書き換える。
-- 戻したときに古い値が蘇らないようにするため。
UPDATE user_exercises
   SET stimulus = jsonb_set(stimulus, '{SIDE_DELT}', '0.3')
 WHERE id IN ('overhead_press', 'db_shoulder_press', 'hs_pl_iso_shoulder_press')
   AND (stimulus->>'SIDE_DELT')::numeric = 0.5;

UPDATE user_exercises
   SET stimulus = jsonb_set(stimulus, '{BICEPS}', '0.3')
 WHERE id IN ('pull_up', 'hs_pl_iso_row', 'hs_pl_iso_high_row', 'hs_pl_iso_low_row',
              'hs_pl_iso_dy_row', 'hs_pl_iso_wide_pulldown', 'hs_pl_iso_front_pulldown')
   AND (stimulus->>'BICEPS')::numeric = 0.5;

UPDATE user_exercises
   SET stimulus = jsonb_set(stimulus, '{BICEPS}', '0.3')
 WHERE id = 'lat_pulldown'
   AND (stimulus->>'BICEPS')::numeric = 0.4;
