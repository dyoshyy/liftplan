#!/usr/bin/env bash
# リリース履歴を出す。
#
# Cloud Run のリビジョンを事実上のリリース台帳として扱い、
# コミットの件名を突き合わせて読める形にする。Git のタグは打たない。
# 二重管理になるだけで、分かることが増えない。
set -euo pipefail

PROJECT="${PROJECT:-liftplan-85309}"
REGION="${REGION:-asia-southeast1}"
SERVICE="${SERVICE:-liftplan-server}"

# 稼働中のリビジョンは1回だけ引く。行ごとに問い合わせると遅い。
active=$(gcloud run services describe "$SERVICE" \
  --region="$REGION" --project="$PROJECT" \
  --format='value(status.traffic[0].revisionName)' 2>/dev/null || true)

printf '%-2s %-30s %-17s %-9s %s\n' '' リビジョン デプロイ時刻 コミット 件名

# csv で出すのは、value だと空のラベルが詰まって列がずれるため。
gcloud run revisions list \
  --service="$SERVICE" --region="$REGION" --project="$PROJECT" \
  --format='csv[no-heading](metadata.name,status.conditions[0].lastTransitionTime.date("%Y-%m-%d %H:%M"),metadata.labels.commit)' \
  2>/dev/null |
while IFS=',' read -r name time commit; do
  mark='  '
  [ "$name" = "$active" ] && mark='->'

  if [ -n "${commit:-}" ]; then
    short="${commit:0:7}"
    subject=$(git log -1 --format='%s' "$commit" 2>/dev/null || echo '（このリポジトリに無いコミット）')
  else
    short='-'
    subject='（自動デプロイ導入前のリビジョン）'
  fi

  printf '%-2s %-30s %-17s %-9s %s\n' "$mark" "$name" "$time" "$short" "$subject"
done
