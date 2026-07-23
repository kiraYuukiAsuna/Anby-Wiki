#!/bin/sh

die() {
  echo "$1" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "backup: missing required command: $1"
}

require_non_production() {
  for value in "${ENV:-}" "${APP_ENV:-}" "${ANBY_ENV:-}"; do
    case "$value" in
      production|Production|PRODUCTION|prod|Prod|PROD)
        die "backup: refusing destructive restore operation in production environment"
        ;;
    esac
  done
}

require_drill_environment() {
  require_non_production
  [ "${RESTORE_ENVIRONMENT:-}" = "drill" ] ||
    die "backup: RESTORE_ENVIRONMENT=drill is required for restore operations"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

verify_checksum_file() {
  checksum_file=$1
  expected=$(awk 'NR == 1 {print $1}' "$checksum_file")
  referenced=$(awk 'NR == 1 {print $2}' "$checksum_file")
  [ -n "$expected" ] && [ -n "$referenced" ] ||
    die "backup: invalid checksum file: $checksum_file"
  actual=$(sha256_file "$(dirname "$checksum_file")/$referenced")
  [ "$actual" = "$expected" ] ||
    die "backup: SHA256 mismatch for $referenced"
}

now_epoch() {
  date +%s
}

elapsed_seconds() {
  started=$1
  finished=$2
  echo $((finished - started))
}

write_object_manifest() {
  object_dir=$1
  output=$2
  (
    cd "$object_dir"
    find . -type f -print | LC_ALL=C sort | while IFS= read -r path; do
      case "$path" in
        *"	"*)
          die "backup: object key contains an unsupported tab character"
          ;;
      esac
      hash=$(sha256_file "$path")
      size=$(wc -c <"$path" | tr -d '[:space:]')
      printf '%s\t%s\t%s\n' "$hash" "$size" "$path"
    done
  ) >"$output"
}

verify_object_manifest() {
  object_dir=$1
  manifest=$2
  regenerated=$(mktemp "${TMPDIR:-/tmp}/anby-object-manifest.XXXXXX")
  write_object_manifest "$object_dir" "$regenerated"
  if ! cmp "$manifest" "$regenerated" >/dev/null; then
    rm -f "$regenerated"
    die "backup: object manifest content or SHA256 mismatch"
  fi
  rm -f "$regenerated"
}
