#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

plugin_id="cpa-codex-quota-stats"
out_dir="${1:-dist}"
version="${PLUGIN_VERSION:-}"

if [[ -z "${version}" ]]; then
  version="$(sed -n 's/^[[:space:]]*var pluginVersion = "\([^"]*\)".*/\1/p' types.go | head -n 1)"
fi
if [[ ! "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([+-][0-9A-Za-z.-]+)?$ ]]; then
  echo "PLUGIN_VERSION must be a release version, got: ${version:-<empty>}" >&2
  exit 1
fi

goos="$(go env GOOS)"
goarch="$(go env GOARCH)"
ext="so"
case "${goos}" in
  darwin) ext="dylib" ;;
  windows) ext="dll" ;;
esac

mkdir -p "${out_dir}"
out_dir="$(cd "${out_dir}" && pwd)"
artifact="${plugin_id}.${ext}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
zip_name="${plugin_id}_${version}_${goos}_${goarch}.zip"

CGO_ENABLED=1 go build -trimpath -buildvcs=false -buildmode=c-shared \
  -ldflags="-s -w -X main.pluginVersion=${version}" \
  -o "${tmp_dir}/${artifact}" .

if command -v zip >/dev/null 2>&1; then
  (cd "${tmp_dir}" && zip -9 -q "${zip_name}" "${artifact}")
else
  python3 - "${tmp_dir}" "${artifact}" "${tmp_dir}/${zip_name}" <<'PY'
import pathlib
import sys
import zipfile

source = pathlib.Path(sys.argv[1]) / sys.argv[2]
destination = pathlib.Path(sys.argv[3])
destination.parent.mkdir(parents=True, exist_ok=True)
with zipfile.ZipFile(destination, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
    archive.write(source, source.name)
PY
fi
mv "${tmp_dir}/${zip_name}" "${out_dir}/${zip_name}"

(
  cd "${out_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${zip_name}" > checksums.txt
  else
    shasum -a 256 "${zip_name}" > checksums.txt
  fi
)

echo "Created ${out_dir}/${zip_name}"
echo "Created ${out_dir}/checksums.txt"
