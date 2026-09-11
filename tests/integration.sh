#!/usr/bin/env bash
# integration.sh — lite cross-combine test: nFPM + FPM + Alien
# Arg $1: "deb" atau "rpm"
set -euo pipefail

FORMAT="${1:-deb}"
DIST="/dist"
CONVERTED="/dist/converted"
mkdir -p "$DIST" "$CONVERTED"

PASS=0
FAIL=0

ok()   { echo "[PASS] $*"; PASS=$((PASS+1)); }
fail() { echo "[FAIL] $*"; FAIL=$((FAIL+1)); }

banner() { echo; echo "=== $* ==="; echo; }

# ─────────────────────────────────────────────
banner "1. DETECT TOOLS"
# ─────────────────────────────────────────────
for tool in nfpm fpm alien; do
  if command -v "$tool" &>/dev/null; then
    ok "$tool: $(command -v $tool)"
  else
    fail "$tool tidak ditemukan"
  fi
done

# ─────────────────────────────────────────────
banner "2. NFPM — build paket native ($FORMAT)"
# ─────────────────────────────────────────────
# Siapkan binary yang akan di-package
install -Dm755 /testdata/hello-unipack /tmp/stagedir/usr/bin/hello-unipack

# Generate nfpm.yaml dari testdata
cat > /tmp/nfpm.yaml <<EOF
name: hello-unipack
version: "1.0.0"
release: "1"
arch: amd64
platform: linux
maintainer: "Unipack Test <test@example.com>"
description: |
  Hello World test package for unipack cross-combine test
homepage: "https://example.com"
license: MIT

contents:
  - src: /tmp/stagedir/usr/bin/hello-unipack
    dst: /usr/bin/hello-unipack

depends:
  - curl
EOF

nfpm pkg --packager "$FORMAT" --config /tmp/nfpm.yaml --target "$DIST"

# Temukan file output
PKG_FILE=$(ls "$DIST"/*."$FORMAT" 2>/dev/null | head -1)
if [[ -n "$PKG_FILE" ]]; then
  ok "nFPM hasilkan: $(basename $PKG_FILE)"
else
  fail "nFPM tidak hasilkan file $FORMAT"
  exit 1
fi

# Verifikasi paket valid
banner "3. VERIFIKASI PAKET NATIVE"
if [[ "$FORMAT" == "deb" ]]; then
  dpkg-deb -I "$PKG_FILE" && ok "dpkg-deb -I valid"
elif [[ "$FORMAT" == "rpm" ]]; then
  rpm -qip "$PKG_FILE" && ok "rpm -qip valid"
fi

# ─────────────────────────────────────────────
banner "4. FPM — build paket native ($FORMAT) via FPM sebagai fallback"
# ─────────────────────────────────────────────
FPM_OUT="$DIST/fpm-out"
mkdir -p "$FPM_OUT"

fpm -s dir -t "$FORMAT" \
  -n hello-unipack-fpm \
  -v 1.0.0 \
  --iteration 1 \
  --description "hello-unipack built by FPM" \
  --url "https://example.com" \
  --maintainer "Unipack Test <test@example.com>" \
  --license MIT \
  -d curl \
  -p "$FPM_OUT/NAME-VERSION_ARCH.EXTENSION" \
  --architecture x86_64 \
  /tmp/stagedir/usr/bin/hello-unipack=/usr/bin/hello-unipack-fpm

FPM_PKG=$(ls "$FPM_OUT"/*."$FORMAT" 2>/dev/null | head -1)
if [[ -n "$FPM_PKG" ]]; then
  ok "FPM hasilkan: $(basename $FPM_PKG)"
else
  fail "FPM tidak hasilkan file $FORMAT"
fi

# ─────────────────────────────────────────────
banner "5. ALIEN — cross-convert ($FORMAT → target lain)"
# ─────────────────────────────────────────────
if [[ "$FORMAT" == "deb" ]]; then
  TARGET_FMT="rpm"
  VERIFY_CMD="rpm -qip"
elif [[ "$FORMAT" == "rpm" ]]; then
  TARGET_FMT="deb"
  VERIFY_CMD="dpkg-deb -I"
fi

# Alien jalankan dari CONVERTED dir (output ditulis ke CWD)
cd "$CONVERTED"
alien --to-"$TARGET_FMT" --keep-version "$PKG_FILE" && ok "alien konversi $FORMAT → $TARGET_FMT"

ALIEN_OUT=$(ls "$CONVERTED"/*."$TARGET_FMT" 2>/dev/null | head -1)
if [[ -n "$ALIEN_OUT" ]]; then
  ok "Alien hasilkan: $(basename $ALIEN_OUT)"
  $VERIFY_CMD "$ALIEN_OUT" && ok "$VERIFY_CMD valid pada hasil alien"
else
  fail "Alien tidak hasilkan file $TARGET_FMT"
fi

# ─────────────────────────────────────────────
banner "HASIL AKHIR"
# ─────────────────────────────────────────────
echo "PASS: $PASS"
echo "FAIL: $FAIL"
echo
ls -lh "$DIST"/ "$CONVERTED"/ 2>/dev/null || true

if [[ $FAIL -gt 0 ]]; then
  echo "GAGAL: $FAIL test tidak lulus"
  exit 1
fi
echo "SEMUA TEST PASS — FORMAT=$FORMAT — nFPM + FPM + Alien cross-combine OK"
