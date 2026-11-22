#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 || ! $1 =~ ^https?://[^/?#]+(/c2)?/?$ ]]; then
  echo "Usage: $0 http://JOBDB_HOST:9047[/c2]" >&2
  exit 2
fi
jobdb=${1%/}
jobdb=${jobdb%/c2}

cortex=${CORTEX_URL:-http://db.lab.c2.run:8081}
job='3MQkO06m6wMX4KpIaKoT0ftLCum'
review='e960c8e9-5c54-4a91-a609-f042d2b5b6e6'
document='review/backstop-test-results.md'

conversation="${cortex%/}/api/projects/c2/jobs/$job/conversation"
printf 'Resolving %s on %s (untimed)\n' "$document" "$cortex"

# A review can span multiple entries. The reader selects the latest generation.
review_entries() {
  local cursor='' snapshot='' page token
  while :; do
    page=$(curl -fsSG --data-urlencode 'page_size=50' --data-urlencode "page_token=$cursor" "$conversation") || return 1
    token=$(jq -er '.snapshot_token | strings | select(length > 0)' <<< "$page") || return 1
    if [[ -n $snapshot && $token != "$snapshot" ]]; then
      echo 'Conversation snapshot changed during lookup' >&2
      return 1
    fi
    snapshot=$token
    jq -c --arg review "$review" '
      .snapshot_token as $snapshot | .entries[]
      | select(.form_fields.presentation.type == "review/v2" and .form_fields.request.review_id == $review)
      | {entry_id, snapshot: $snapshot, generation: (.locator.generation_ordinal // 0)}' <<< "$page" || return 1
    cursor=$(jq -r '.next_page_token // ""' <<< "$page") || return 1
    [[ -n $cursor ]] || break
  done
}
selection=$(review_entries | jq -ser '
  if length == 0 then error("Review entry not found")
  else (sort_by(-.generation) | first | [.entry_id, .snapshot] | @tsv) end') || exit 1
IFS=$'\t' read -r entry snapshot <<< "$selection"

coordinates=$(
  curl -fsSG --data-urlencode "entry_id=$entry" --data-urlencode "snapshot_token=$snapshot" "$conversation/entry" |
    jq -er --arg doc "$document" '
      def identity:
        if .kind == "stored" then ["stored", .stored.key.jobId, .stored.key.taskOrdinal, .stored.key.name]
        elif .kind == "attached" then ["attached", .name]
        else null end;
      (.raw_form.request.documents[$doc] // error("Review has no such document")) as $ref
      | ($ref | identity) as $identity
      | ("/request/documents/" + ($doc | gsub("~"; "~0") | gsub("/"; "~1"))) as $pointer
      | [.request_manifest[] | select(
          ($identity != null and (((.original_ref // .ref) | identity) == $identity or (.ref | identity) == $identity))
          or ($ref.kind != "stored" and any(.associations[]?; .source == $pointer)))]
      | unique_by(.artifact_id)
      | if length != 1 then error("Expected one matching manifest member, found \(length)") else .[0] end
      | select(.availability == "retained" and .ref.kind == "stored")
      | .ref.stored.key as $key
      | select($key.jobId != null and $key.taskOrdinal != null and $key.name != null)
      | [.artifact_id, ($key.jobId | @uri), $key.taskOrdinal, ($key.name | @uri)] | @tsv'
) || { echo "Could not resolve the retained artifact for $document" >&2; exit 1; }

IFS=$'\t' read -r artifact producer ordinal name <<< "$coordinates"
jobs="$jobdb/v1/tenants/c2/jobs"
url="$jobs/$producer/chapters/$ordinal/artifacts/$name"
printf 'Direct artifact GET %s\n' "$url"

failed=0
measure() {
  printf '%-24s ' "$1"
  shift
  curl -fsS -o /dev/null \
    -w 'HTTP %{http_code}  bytes=%{size_download}  dns=%{time_namelookup}s  connect=%{time_connect}s  first_byte=%{time_starttransfer}s  total=%{time_total}s\n' \
    "$@" || failed=1
}

measure 'Cortex document' "${cortex%/}/api/projects/c2/jobs/$producer/stored-document?ordinal=$ordinal&name=$name"
measure 'Cortex legacy history' -G --data-urlencode "entry_id=$entry" --data-urlencode "artifact_id=$artifact" \
  --data-urlencode "snapshot_token=$snapshot" "$conversation/artifact"
measure 'JobDB artifact' "$url"
measure 'JobDB review chapters' "$jobs/$job/chapters?startOrdinal=0"
if [[ $producer != "$job" ]]; then
  measure 'JobDB producer chapters' "$jobs/$producer/chapters?startOrdinal=0"
fi
exit "$failed"
