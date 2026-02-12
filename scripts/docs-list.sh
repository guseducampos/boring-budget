#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCS_DIR="$ROOT_DIR/docs"

if [[ ! -d "$DOCS_DIR" ]]; then
  echo "docs directory not found at $DOCS_DIR" >&2
  exit 1
fi

echo "Listing markdown docs with summary/read_when metadata:"

while IFS= read -r file; do
  rel_path="${file#"$ROOT_DIR/"}"

  metadata=$(awk '
    BEGIN { in_fm=0; saw_fm=0; in_read_when=0; summary=""; read_when=""; sep="" }
    NR==1 && $0=="---" { in_fm=1; saw_fm=1; next }
    in_fm && $0=="---" { in_fm=0; next }
    in_fm {
      if ($0 ~ /^summary:[[:space:]]*/) {
        line=$0
        sub(/^summary:[[:space:]]*/, "", line)
        gsub(/^"|"$/, "", line)
        summary=line
        next
      }
      if ($0 ~ /^read_when:[[:space:]]*$/) {
        in_read_when=1
        next
      }
      if (in_read_when && $0 ~ /^[[:space:]]*-[[:space:]]+/) {
        line=$0
        sub(/^[[:space:]]*-[[:space:]]+/, "", line)
        read_when=read_when sep line
        sep="; "
        next
      }
      if (in_read_when && $0 !~ /^[[:space:]]*$/ && $0 !~ /^[[:space:]]*-/) {
        in_read_when=0
      }
    }
    END {
      if (!saw_fm) {
        print "NO_FRONT_MATTER"
      } else {
        print "SUMMARY=" summary
        print "READ_WHEN=" read_when
      }
    }
  ' "$file")

  if [[ "$metadata" == "NO_FRONT_MATTER" ]]; then
    echo "$rel_path - [missing front matter]"
    continue
  fi

  summary=$(printf '%s\n' "$metadata" | sed -n 's/^SUMMARY=//p' | head -n1)
  read_when=$(printf '%s\n' "$metadata" | sed -n 's/^READ_WHEN=//p' | head -n1)

  if [[ -z "$summary" ]]; then
    summary="[summary missing]"
  fi

  echo "$rel_path - $summary"
  if [[ -n "$read_when" ]]; then
    echo "  Read when: $read_when"
  fi
done < <(find "$DOCS_DIR" -type f -name '*.md' | sort)

echo
echo "Reminder: if your change affects behavior, update docs/SPEC.md or docs/contracts/* in the same change."
