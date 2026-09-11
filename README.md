# UniPack — Universal Package Manager Builder

UniPack adalah *wrapper* pemaketan universal untuk Linux. Tool ini menggabungkan kekuatan 4 alat pemaketan populer menjadi satu alur kerja yang seragam.

**Gabungan Tools:**
- **nFPM** — Builder utama (cepat, Go-based)
- **FPM** — Builder cadangan untuk format edge-case
- **Pacstall** — Sistem build pipeline dari source code
- **Alien** — Converter format antar-binary (last resort)

Satu perintah untuk mem-build paket `.deb`, `.rpm`, `.apk`, `.pkg.tar.zst`, dan banyak lagi, dari satu format file deklarasi: **UniScript**.

---

## 🛠️ Prasyarat (Requirements)

UniPack **tidak membutuhkan** ke-4 tools diinstall secara manual. Cukup install `unipack`, lalu jalankan `unipack setup` — semua dependensi (nFPM, FPM, Alien, Ruby) akan diinstall otomatis.

Jika ingin install manual:
- `nfpm` — [Install nFPM](https://nfpm.goreleaser.com/docs/install/)
- `fpm` — Install via `gem install fpm`
- `alien` — Debian: `apt install alien`, Fedora: `dnf install alien`
- `bash`, `curl`, `tar`, `make` (Untuk pipeline build dari source)

---

## 🚀 Instalasi

UniPack ditulis dalam Go dan tidak memiliki external library dependencies.

### 1. Build dari Source Code
Pastikan Anda sudah menginstall `go` (min. v1.21).

```bash
git clone https://github.com/unipack/unipack.git
cd unipack
go build -o unipack ./cmd/unipack
sudo mv unipack /usr/local/bin/
```

### 2. Cross-Compile (dari Windows / macOS)
Jika Anda mengembangkan di Windows dan ingin mem-build binary untuk Linux:

```bash
GOOS=linux GOARCH=amd64 go build -o unipack ./cmd/unipack
```

### 3. Setup Otomatis (Install Semua Dependensi)
Setelah `unipack` terpasang, jalankan satu perintah untuk menginstall semua tools yang dibutuhkan:

```bash
sudo unipack setup
```

Perintah ini akan:
- Mendeteksi distro Linux Anda (Debian, Fedora, Arch, Alpine, dll)
- Menginstall paket sistem: `curl`, `tar`, `make`, `gcc`, `ruby`, `ruby-dev`
- Mengunduh dan memasang **nFPM** binary dari GitHub Releases ke `~/.local/share/unipack/bin/` (tanpa butuh `apt`/`dnf`)
- Menginstall **FPM** via `gem install fpm`
- Menginstall **Alien** via package manager OS (`apt install alien` / `dnf install alien`)

Cek apa yang akan diinstall tanpa eksekusi:

```bash
unipack setup --dry-run
```

Setelah setup selesai, verifikasi semua tool tersedia:

```bash
unipack detect
```

---

## 📖 Cara Penggunaan

### 1. Inisialisasi Project (Membuat UniScript)
Generate template `.uniscript` kosong:

```bash
unipack init --name myapp
```
Akan menghasilkan file `myapp.uniscript`.

### 2. Build Paket
Jalankan unipack build. Format target akan di-deteksi otomatis dari OS (misal: jalankan di Ubuntu otomatis `.deb`), atau bisa dipaksa dengan `--format`.

```bash
# Auto detect OS format
unipack build myapp.uniscript --output ./dist

# Paksa ke RPM
unipack build myapp.uniscript --format rpm --output ./dist

# Paksa ke Arch Linux
unipack build myapp.uniscript --format archlinux --output ./dist
```

### 3. Konversi Antar Format (Menggunakan Alien)
Jika Anda hanya memiliki file biner (seperti `.deb`) dan butuh instalasi di Fedora (`.rpm`), Anda bisa men-convertnya secara on-the-fly.

*Peringatan: Gunakan hanya sebagai jalan terakhir. Alien rentan terhadap path hardcoded spesifik distro asal.*

```bash
unipack convert ./dist/myapp_1.0_amd64.deb --to rpm --output ./dist
```

### 4. Cek Sistem
Lihat apakah UniPack berhasil mendeteksi OS distro Anda:

```bash
unipack detect
```

---

## 📄 Format `UniScript`

UniScript adalah format script gabungan (terinspirasi dari PKGBUILD di Arch Linux dan `pacscript` dari Pacstall). File ini berbentuk bash script dengan array konfigurasi dan blok lifecycle fungsi.

Contoh `hello.uniscript` (Instalasi binary tanpa kompilasi):

```bash
pkgname="hello-unipack"
pkgver="1.0.0"
pkgrel="1"
pkgdesc="Hello World package"
url="https://example.com"
license=("MIT")
maintainer=("Nama Anda <email@anda.com>")
arch=("amd64")

# Menggunakan nama paket generik ("openssl", "curl").
# UniPack otomatis menerjemahkan ke "libssl-dev" (Ubuntu) atau "openssl-devel" (Fedora).
depends=("curl")

# Blok fungsi package (Dieksekusi dengan Bash)
package() {
  cd "${pkgname}-${pkgver}"
  # Install binary pre-built ke struktur filesystem packaging
  install -Dm755 "hello-unipack" -t "${pkgdir}/usr/bin"
}
```

Jika paket butuh di-compile (C/C++, Rust, Go, dll), tambahkan URL source code dan fungsi `prepare()`, `build()`, dan `check()`.

---

## 🧪 Testing

Project ini menggunakan Podman untuk *lite integration testing* di 2 OS sekaligus (Debian dan Fedora).

Jalankan test:
```bash
make test
```
Ini akan membuktikan bahwa alur `nFPM -> FPM fallback -> Alien Convert` berhasil memproduksi paket `.deb` dan `.rpm` yang valid.
