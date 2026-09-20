#!/bin/sh
# Installs a verified binary, or runs it once with --run. No sudo or shell edits.
set -eu

main() {
  repo='LeifWebber/music-unlock'
  version='latest'
  bin_dir="${HOME}/.local/bin"
  run_once=false
  force=false
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --version) [ "$#" -ge 2 ] || { echo '--version needs a value' >&2; return 2; }; version=$2; shift 2 ;;
      --bin-dir) [ "$#" -ge 2 ] || { echo '--bin-dir needs a directory' >&2; return 2; }; bin_dir=$2; shift 2 ;;
      --force) force=true; shift ;;
      --run) run_once=true; shift; [ "${1-}" != '--' ] || shift; break ;;
      -h|--help) echo 'Usage: install.sh [--version vX.Y.Z] [--bin-dir DIR] [--force] [--run [--] CLI_ARGS...]'; return ;;
      *) echo "Unknown installer option: $1" >&2; return 2 ;;
    esac
  done
  case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo 'Windows 请使用 install.ps1 或 install.cmd，参见 README。' >&2; return 1 ;; esac
  case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) echo 'Unsupported CPU architecture' >&2; return 1 ;; esac
  command -v curl >/dev/null || { echo 'curl is required' >&2; return 1; }
  temp=$(mktemp -d "${TMPDIR:-/tmp}/music-unlock.XXXXXXXX")
  trap 'rm -rf "$temp"' EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM
  if [ "$version" = latest ]; then
    version=$(curl --proto '=https' --tlsv1.2 -fsSL --retry 2 -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")
    version=${version##*/}
  fi
  # Tags become URL/path components; reject separators and shell metacharacters.
  case "$version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo 'Invalid release version (expected vX.Y.Z)' >&2; return 2 ;; esac
  case "$version" in *[!A-Za-z0-9.+-]*) echo 'Invalid release version' >&2; return 2 ;; esac
  name="music-unlock-$version-$os-$arch"
  archive="$name.tar.gz"
  base="https://github.com/$repo/releases/download/$version"
  curl --proto '=https' --tlsv1.2 -fsSL --retry 2 "$base/$archive" -o "$temp/$archive"
  curl --proto '=https' --tlsv1.2 -fsSL --retry 2 "$base/SHA256SUMS" -o "$temp/SHA256SUMS"
  expected=$(awk -v name="$archive" '$2 == name {print $1}' "$temp/SHA256SUMS")
  [ "${#expected}" -eq 64 ] || { echo 'Missing or ambiguous SHA256 checksum' >&2; return 1; }
  if command -v sha256sum >/dev/null; then
    actual=$(sha256sum "$temp/$archive" | awk '{print $1}')
  else
    actual=$(shasum -a 256 "$temp/$archive" | awk '{print $1}')
  fi
  [ "$actual" = "$expected" ] || { echo 'SHA256 verification failed' >&2; return 1; }
  # Only extract the expected executable, never arbitrary archive paths.
  tar -xzf "$temp/$archive" -C "$temp" "$name/unmus"
  executable="$temp/$name/unmus"
  [ -f "$executable" ] && [ ! -L "$executable" ] || { echo 'Invalid executable in archive' >&2; return 1; }
  chmod 755 "$executable"
  if [ "$run_once" = true ]; then
    "$executable" "$@"
  else
    mkdir -p "$bin_dir"
    if [ -e "$bin_dir/unmus" ] || [ -L "$bin_dir/unmus" ]; then
      if [ "$force" != true ] || [ ! -f "$bin_dir/unmus" ] || [ -L "$bin_dir/unmus" ]; then
        printf '未覆盖已有路径：%s/unmus。请使用 --bin-dir 选择其他目录；确认替换普通文件时可加 --force。\n' "$bin_dir" >&2
        return 1
      fi
    fi
    staged=$(mktemp "$bin_dir/.music-unlock.XXXXXXXX")
    if ! cp "$executable" "$staged" || ! chmod 755 "$staged" || ! mv -f "$staged" "$bin_dir/unmus"; then
      rm -f "$staged"; return 1
    fi
    printf '已安装：%s/unmus\n' "$bin_dir"
    case ":$PATH:" in *":$bin_dir:"*) ;; *) printf '请将 %s 加入 PATH，或使用完整路径运行。\n' "$bin_dir" ;; esac
  fi
}

main "$@"
