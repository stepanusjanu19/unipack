# Universal Linux Package Manager — Build Plan

Gabungan nFPM + FPM + Pacstall + Alien menjadi satu toolchain pemaketan lintas distro.

---

## Tujuan

Satu perintah (`unipack`) hasilkan paket native (`.deb`/`.rpm`/`.apk`/`.pkg.tar.zst`) dari:
- **Source code** (via build script), atau
- **Binary yang sudah ada**, atau
- **Paket format lain** (konversi lintas format)

---

## Peran Masing-Masing Tool

| Tool | Peran dalam sistem | Bahasa | Kondisi dipanggil |
|---|---|---|---|
| **Pacstall** | Frontend — baca script, unduh source, kompilasi | Bash | Ada source / `pacscript` tersedia |
| **nFPM** | Builder utama — bungkus binary jadi paket native | Go | Setelah kompilasi selesai, atau input binary |
| **FPM** | Builder cadangan — format langka / edge case | Ruby | nFPM tidak support format target |
| **Alien** | Converter — ubah paket biner lintas format | Perl | Tidak ada source, hanya ada paket format lain |

---

## Arsitektur Alur Kerja

```
unipack install <pkg>
       │
       ▼
[1. RESOLVE] Cek repository script (pacscript/PKGBUILD)
       │
       ├── Ada script? ──► [2A. BUILD PATH]
       │                        │
       │                   Pacstall: unduh source, install makedepends
       │                        │
       │                   prepare() → build() → check() → package()
       │                        │
       │                   nFPM: bungkus hasil ke format target
       │                   (fallback: FPM jika format tidak didukung nFPM)
       │
       └── Tidak ada script? ──► [2B. CONVERT PATH]
                                      │
                                 Cari paket pre-built format lain
                                      │
                                 Alien: konversi ke format native OS
       │
       ▼
[3. INSTALL] Serahkan ke package manager native (apt / dnf / pacman / apk)
```

---

## Struktur Script Pemaketan (UniScript)

Format gabungan yang menyatukan field Pacscript + nFPM config:

```bash
# === METADATA ===
pkgname="foo"
pkgver="1.0.0"
pkgrel="1"
epoch=""
pkgdesc="Deskripsi singkat paket"
url="https://upstream.example.com"
bugs="https://upstream.example.com/issues"
license=("MIT")
maintainer=("Nama <email@example.com>")
arch=("amd64" "arm64" "any")

# === SUMBER ===
source=("https://github.com/example/foo/archive/refs/tags/${pkgver}.tar.gz")
sha256sums=("abc123...")

# === DEPENDENSI ===
# Mapping per-distro: key = nama generik, value = nama per format
depends=(
  "openssl"        # generik; di-resolve ke libssl-dev / openssl-devel / openssl
)
makedepends=("gcc" "make")
checkdepends=("pytest")
optdepends=("bar: fitur opsional")
pacdeps=("dmenu")   # khusus Pacstall

# === KONFLIK / RELASI ===
conflicts=("foo-git" "foo-bin")
replaces=("foo-old")
provides=("foo")
breaks=("libfoo-git")

# === KOMPATIBILITAS ===
compatible=("*:jammy" "*:noble" "fedora:40")
incompatible=("debian:stretch")

# === FILE & INSTALASI (nFPM style) ===
# Didefinisikan dalam nfpm.yaml yang di-generate otomatis
contents:
  - src: "build/bin/foo"
    dst: "/usr/bin/foo"
  - src: "config/foo.conf"
    dst: "/etc/foo/foo.conf"
    type: config|noreplace

# === LIFECYCLE SCRIPTS ===
scripts:
  preinstall: "./scripts/preinstall.sh"
  postinstall: "./scripts/postinstall.sh"
  preremove: "./scripts/preremove.sh"
  postremove: "./scripts/postremove.sh"

# === OVERRIDES PER FORMAT (nFPM style) ===
overrides:
  deb:
    depends:
      - "libssl-dev (>= 1.1.0)"
  rpm:
    depends:
      - "openssl-devel >= 1.1.0"
  apk:
    depends:
      - "openssl"
  archlinux:
    depends:
      - "openssl"

# === BUILD FUNCTIONS (Pacscript / PKGBUILD style) ===
prepare() {
  cd "${pkgname}-${pkgver}"
  ./autogen.sh
  ./configure --prefix=/usr
}

build() {
  cd "${pkgname}-${pkgver}"
  make -j"${NCPU}"
}

check() {
  cd "${pkgname}-${pkgver}"
  make test
}

package() {
  cd "${pkgname}-${pkgver}"
  make install DESTDIR="${pkgdir}"
}
```

---

## Dependency Mapping Table (Nama Generik → Per Distro)

Tantangan terbesar. Harus ada kamus mapping:

| Nama Generik | Debian/Ubuntu | Fedora/RHEL | Arch Linux | Alpine |
|---|---|---|---|---|
| `openssl` | `libssl-dev` | `openssl-devel` | `openssl` | `openssl-dev` |
| `zlib` | `zlib1g-dev` | `zlib-devel` | `zlib` | `zlib-dev` |
| `sqlite` | `libsqlite3-dev` | `sqlite-devel` | `sqlite` | `sqlite-dev` |
| `python3` | `python3` | `python3` | `python` | `python3` |
| `libcurl` | `libcurl4-openssl-dev` | `libcurl-devel` | `curl` | `curl-dev` |
| `libxml2` | `libxml2-dev` | `libxml2-devel` | `libxml2` | `libxml2-dev` |

> **Sumber mapping:** distro package search API + database komunitas (seperti Repology).

---

## Format Output yang Didukung

| Format | Tool | Target Distro |
|---|---|---|
| `.deb` | nFPM (utama) / FPM / Alien | Debian, Ubuntu, Mint |
| `.rpm` | nFPM (utama) / FPM / Alien | Fedora, RHEL, openSUSE |
| `.apk` | nFPM | Alpine Linux |
| `.pkg.tar.zst` | nFPM (archlinux) | Arch Linux, Manjaro |
| `.ipk` | nFPM | OpenWrt |
| `.tgz` (Slackware) | FPM / Alien | Slackware |
| `.txz` | FPM | Slackware modern |

---

## Sumber Data nFPM Config Fields

Dari dokumentasi nFPM (`nfpm.yaml`):

```yaml
name: foo
version: "1.0.0"
release: "1"
epoch: ""
prerelease: ""
arch: amd64
platform: linux
maintainer: "Nama <email>"
description: "Deskripsi paket"
homepage: "https://example.com"
license: MIT
mtime: "2009-11-10T23:00:00Z"

contents:
  - src: ./bin/foo
    dst: /usr/bin/foo

scripts:
  preinstall: ./scripts/preinstall.sh
  postinstall: ./scripts/postinstall.sh
  preremove: ./scripts/preremove.sh
  postremove: ./scripts/postremove.sh

overrides:
  deb:
    depends: []
  rpm:
    depends: []
  apk:
    depends: []
  archlinux:
    depends: []
```

---

## Sumber Data Pacscript Fields

Dari dokumentasi Pacstall Wiki:

| Field | Fungsi |
|---|---|
| `pkgname` | Nama paket |
| `pkgver` | Versi upstream |
| `pkgrel` | Revisi pacscript |
| `epoch` | Paksa versi lebih baru |
| `pkgdesc` | Deskripsi |
| `url` | Homepage |
| `bugs` | Bug tracker |
| `source[]` | URL sumber (tarball/zip/git) |
| `{sha256,b2,md5}sums[]` | Checksum sumber |
| `arch[]` | Arsitektur (`amd64`, `arm64`, `any`) |
| `license[]` | Lisensi (SPDX) |
| `depends[]` | Runtime dependency |
| `makedepends[]` | Build-time dependency |
| `checkdepends[]` | Test dependency |
| `optdepends[]` | Opsional + deskripsi |
| `pacdeps[]` | Dependency dari Pacstall |
| `conflicts[]` | Tidak bisa install bersamaan |
| `breaks[]` | Paket yang rusak jika ini diinstall |
| `replaces[]` | Gantikan paket lain |
| `provides[]` | Virtual package |
| `gives` | Override nama paket output |
| `compatible[]` | Distro/versi yang support |
| `incompatible[]` | Distro/versi yang tidak support |
| `backup[]` | File config yang di-preserve saat upgrade |
| `priority` | dpkg priority (`essential`, `optional`, dll) |
| `maintainer[]` | Maintainer list |
| `mask[]` | Cegah apt install paket dengan nama sama |
| `prepare()` | Pre-build (autogen, configure) |
| `build()` | Kompilasi |
| `check()` | Testing |
| `package()` | Install ke `$pkgdir` |

---

## Kondisi Penggunaan Alien

Alien dipakai **hanya jika**:
1. Tidak ada `pacscript` / source tersedia
2. Hanya ada paket binary format lain (misal `.rpm` dari vendor proprietary)
3. Target OS berbeda format dengan paket yang tersedia

**Peringatan:** Alien rentan gagal jika paket source punya:
- Hardcoded path ke direktori spesifik distro
- Post-install script yang bergantung pada systemd unit distro asal
- Dependency name yang tidak di-resolve otomatis

Gunakan Alien sebagai **last resort**, lalu verifikasi manual hasil konversinya.

---

## Struktur Direktori Proyek

```
unipack/
├── cmd/
│   └── unipack/
│       └── main.go           # Entry point (Go)
├── internal/
│   ├── resolver/
│   │   ├── script.go         # Baca & parse UniScript
│   │   └── depmap.go         # Dependency name mapping
│   ├── builder/
│   │   ├── nfpm.go           # Wrapper nFPM (utama)
│   │   ├── fpm.go            # Wrapper FPM (cadangan)
│   │   └── pacstall.go       # Wrapper Pacstall (build from source)
│   ├── converter/
│   │   └── alien.go          # Wrapper Alien (konversi format)
│   └── detector/
│       └── distro.go         # Deteksi OS/distro saat runtime
├── data/
│   └── depmap.json           # Kamus mapping dependency lintas distro
├── scripts/
│   └── templates/
│       ├── nfpm.yaml.tmpl    # Template nFPM config
│       └── uniscript.tmpl    # Template UniScript kosong
├── go.mod
├── go.sum
└── README.md
```

---

## Langkah Implementasi

- [ ] **Fase 1: Resolver & Deteksi**
  - Deteksi distro runtime (`/etc/os-release`)
  - Parse UniScript format
  - Build dependency mapping database (Repology API atau static JSON)

- [ ] **Fase 2: Builder Integration**
  - Wrapper nFPM sebagai Go library (sudah tersedia: `github.com/goreleaser/nfpm/v2`)
  - Wrapper FPM via subprocess (Ruby, perlu `gem install fpm`)
  - Wrapper Pacstall build pipeline (Bash subprocess)

- [ ] **Fase 3: Converter Integration**
  - Wrapper Alien via subprocess (Perl, perlu `alien` binary)
  - Validasi hasil konversi (cek dependency & file list)

- [ ] **Fase 4: CLI & UX**
  - `unipack build` — build dari UniScript
  - `unipack convert <file>` — konversi paket
  - `unipack init` — generate UniScript template kosong
  - `unipack install <pkg>` — full pipeline install

- [ ] **Fase 5: Repository**
  - Format repository pusat untuk UniScript
  - API search paket
  - Submit / update script

---

## Referensi

- nFPM docs: https://nfpm.goreleaser.com/docs/
- nFPM config ref: https://nfpm.goreleaser.com/docs/configuration/
- FPM docs: https://fpm.readthedocs.io/en/latest/
- FPM repo: https://github.com/jordansissel/fpm
- Pacstall wiki: https://github.com/pacstall/pacstall/wiki/Pacscript-101
- Alien repo: https://github.com/mildred/alien
- Repology API: https://repology.org/api/v1/
