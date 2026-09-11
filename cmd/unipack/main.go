// cmd/unipack/main.go — CLI entry point untuk unipack.
//
// Subcommands:
//   unipack setup [--dry-run]          — install nFPM, FPM, Alien, Ruby otomatis
//   unipack build <script.uniscript> [--format deb|rpm|apk|...] [--output ./dist]
//   unipack convert <package-file> --to deb|rpm|tgz [--output ./dist]
//   unipack init [--name foo] [--output ./foo.uniscript]
//   unipack detect
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stepanusjanu19/unipack/internal/builder"
	"github.com/stepanusjanu19/unipack/internal/converter"
	"github.com/stepanusjanu19/unipack/internal/detector"
	"github.com/stepanusjanu19/unipack/internal/resolver"
	"github.com/stepanusjanu19/unipack/internal/setup"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		cmdSetup(os.Args[2:])
	case "build":
		cmdBuild(os.Args[2:])
	case "convert":
		cmdConvert(os.Args[2:])
	case "init":
		cmdInit(os.Args[2:])
	case "detect":
		cmdDetect()
	case "version", "--version", "-v":
		fmt.Printf("unipack %s\n", version)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "subcommand tidak dikenal: %s\n", os.Args[1])
		printHelp()
		os.Exit(1)
	}
}

// cmdSetup: install otomatis nFPM, FPM, Alien, dan dependensi sistem.
func cmdSetup(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "laporkan apa yang akan dilakukan tanpa eksekusi")
	fs.Parse(args)

	fmt.Println("[unipack] setup: cek & install tools yang dibutuhkan")
	fmt.Println()

	// Ensure ToolDir ada di PATH
	if err := setup.EnsurePATH(); err != nil {
		fmt.Fprintf(os.Stderr, "[unipack] warning: tidak bisa menambah ToolDir ke PATH: %v\n", err)
	}

	results := setup.Run(*dryRun)

	pass, skip, fail := 0, 0, 0
	for _, r := range results {
		switch {
		case r.Error != nil:
			fmt.Printf("  [FAIL] %-12s %v\n", r.Tool, r.Error)
			fail++
		case r.Skipped:
			fmt.Printf("  [OK]   %-12s sudah terinstall\n", r.Tool)
			skip++
		case r.Installed:
			fmt.Printf("  [INST] %-12s berhasil diinstall\n", r.Tool)
			pass++
		}
	}

	fmt.Println()
	fmt.Printf("Terinstall: %d  Sudah ada: %d  Gagal: %d\n", pass, skip, fail)

	if fail > 0 {
		fmt.Fprintln(os.Stderr, "[unipack] setup tidak lengkap. Install manual tool yang gagal.")
		os.Exit(1)
	}

	fmt.Println("[unipack] setup selesai. Semua tools siap.")
}

// cmdBuild: build paket dari UniScript.
func cmdBuild(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	format := fs.String("format", "", "format output: deb, rpm, apk, archlinux, ipk, tgz (default: otomatis dari distro)")
	output := fs.String("output", "./dist", "direktori output paket")
	skipCheck := fs.Bool("skip-check", false, "skip phase check()")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: unipack build <script.uniscript> [flags]")
		os.Exit(1)
	}
	scriptPath := fs.Arg(0)

	// Deteksi distro
	distro, err := detector.Detect()
	must(err, "deteksi distro")

	// Tentukan format output
	targetFormat := *format
	if targetFormat == "" {
		targetFormat = distro.PackageFmt
		if targetFormat == "unknown" {
			die("tidak bisa menentukan format otomatis. Gunakan --format")
		}
	}

	// Parse UniScript
	fmt.Printf("[unipack] parse script: %s\n", scriptPath)
	script, err := resolver.ParseFile(scriptPath)
	must(err, "parse UniScript")

	// Cek kompatibilitas distro
	if err := checkCompatibility(script, distro); err != nil {
		die(err.Error())
	}

	// Buat output dir
	must(os.MkdirAll(*output, 0755), "buat output dir")

	// Pilihan build: source-based atau binary wrap
	if hasBuildFunctions(script) {
		// === Jalur A: Build dari source (Pacstall pipeline) ===
		fmt.Println("[unipack] mode: build dari source")

		pb, err := builder.NewPacstall()
		must(err, "init pacstall builder")
		defer pb.Cleanup()

		if *skipCheck {
			script.CheckFn = ""
		}

		result, err := pb.Run(script)
		must(err, "jalankan build pipeline")

		// Bungkus hasil build dengan nFPM
		buildOpts := builder.BuildOptions{
			Script:    script,
			DistroID:  distro.FamilyID(),
			Format:    targetFormat,
			OutputDir: *output,
			PkgDir:    result.PkgDir,
		}
		outFile, err := runBuildWithFallback(buildOpts, targetFormat)
		must(err, "build paket")

		fmt.Printf("[unipack] paket berhasil: %s\n", outFile)
	} else {
		// === Jalur B: Wrap binary yang sudah ada ===
		fmt.Println("[unipack] mode: wrap binary (tidak ada build functions)")

		nb, err := builder.NewNFPM()
		must(err, "nfpm tidak ditemukan. Install: https://nfpm.goreleaser.com/docs/install/")

		buildOpts := builder.BuildOptions{
			Script:    script,
			DistroID:  distro.FamilyID(),
			Format:    targetFormat,
			OutputDir: *output,
		}
		outFile, err := nb.Build(buildOpts)
		must(err, "nfpm build")

		fmt.Printf("[unipack] paket berhasil: %s\n", outFile)
	}
}

// cmdConvert: konversi paket antar format dengan Alien.
func cmdConvert(args []string) {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	toFormat := fs.String("to", "", "format tujuan: deb, rpm, tgz, slp")
	output := fs.String("output", "./dist", "direktori output")
	scripts := fs.Bool("scripts", false, "coba konversi lifecycle scripts")
	verbose := fs.Bool("verbose", false, "verbose output")
	fs.Parse(args)

	if fs.NArg() < 1 || *toFormat == "" {
		fmt.Fprintln(os.Stderr, "usage: unipack convert <package-file> --to deb|rpm|tgz [flags]")
		os.Exit(1)
	}
	inputFile := fs.Arg(0)

	// Deteksi format input
	inputFmt, err := converter.DetectInputFormat(inputFile)
	must(err, "deteksi format input")

	if inputFmt == *toFormat {
		fmt.Printf("[unipack] file sudah dalam format %s, tidak perlu konversi.\n", inputFmt)
		return
	}

	// Peringatan Alien
	fmt.Println("[unipack] PERINGATAN: Alien konversi paket biner — hasilnya mungkin tidak sempurna.")
	fmt.Println("          Verifikasi dependency dan lifecycle scripts setelah konversi.")

	a, err := converter.NewAlien()
	must(err, "alien tidak ditemukan. Install: apt install alien / atau lihat README")

	must(os.MkdirAll(*output, 0755), "buat output dir")

	outFile, err := a.Convert(converter.ConvertOptions{
		InputFile: inputFile,
		OutputDir: *output,
		ToFormat:  *toFormat,
		Scripts:   *scripts,
		Verbose:   *verbose,
	})
	must(err, "alien convert")

	fmt.Printf("[unipack] hasil konversi: %s\n", outFile)
}

// cmdInit: generate template UniScript kosong.
func cmdInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	name := fs.String("name", "mypkg", "nama paket")
	output := fs.String("output", "", "path output (default: ./<name>.uniscript)")
	fs.Parse(args)

	outPath := *output
	if outPath == "" {
		outPath = filepath.Join(".", *name+".uniscript")
	}

	must(os.WriteFile(outPath, []byte(uniscriptTemplate(*name)), 0644), "tulis template")
	fmt.Printf("[unipack] template dibuat: %s\n", outPath)
}

// cmdDetect: tampilkan info distro yang terdeteksi.
func cmdDetect() {
	d, err := detector.Detect()
	must(err, "deteksi distro")
	fmt.Printf("ID:          %s\n", d.ID)
	fmt.Printf("ID_LIKE:     %s\n", d.IDLike)
	fmt.Printf("Version:     %s\n", d.VersionID)
	fmt.Printf("Pretty:      %s\n", d.PrettyName)
	fmt.Printf("Family:      %s\n", d.FamilyID())
	fmt.Printf("PackageFmt:  %s\n", d.PackageFmt)
	fmt.Printf("PackageCmd:  %s\n", d.PackageCmd)
}

// runBuildWithFallback: coba nFPM dulu, fallback ke FPM jika gagal atau format FPM-only.
func runBuildWithFallback(opts builder.BuildOptions, format string) (string, error) {
	if builder.FPMSupports(format) {
		// format hanya FPM
		fb, err := builder.NewFPM()
		if err != nil {
			return "", fmt.Errorf("format %s butuh FPM: %w", format, err)
		}
		return fb.Build(opts)
	}

	// Coba nFPM
	nb, err := builder.NewNFPM()
	if err != nil {
		// nFPM tidak ada, coba FPM
		fmt.Printf("[unipack] nFPM tidak ditemukan, coba FPM: %v\n", err)
		fb, ferr := builder.NewFPM()
		if ferr != nil {
			return "", fmt.Errorf("nFPM dan FPM keduanya tidak tersedia:\n  nFPM: %v\n  FPM: %v", err, ferr)
		}
		return fb.Build(opts)
	}
	return nb.Build(opts)
}

// checkCompatibility verifikasi distro saat ini terhadap compatible/incompatible.
func checkCompatibility(s *resolver.UniScript, d *detector.Distro) error {
	// compatible[] diisi: jika ada entry, hanya distro yang cocok yang lanjut
	if len(s.Compatible) > 0 {
		for _, c := range s.Compatible {
			if matchDistro(c, d) {
				return nil
			}
		}
		return fmt.Errorf("distro %s %s tidak ada dalam compatible list script ini", d.ID, d.VersionID)
	}
	// incompatible[]: jika distro match, tolak
	for _, ic := range s.Incompatible {
		if matchDistro(ic, d) {
			return fmt.Errorf("distro %s %s ada dalam incompatible list script ini", d.ID, d.VersionID)
		}
	}
	return nil
}

// matchDistro cocokkan entry format "*:jammy" atau "debian:stretch" dengan Distro.
func matchDistro(entry string, d *detector.Distro) bool {
	parts := splitEntry(entry)
	distroID := parts[0]
	version := parts[1]

	if distroID != "*" && distroID != d.ID {
		return false
	}
	if version == "*" || version == "" {
		return true
	}
	return version == d.VersionID || version == codename(d.PrettyName)
}

func splitEntry(entry string) [2]string {
	if idx := strings.IndexByte(entry, ':'); idx >= 0 {
		return [2]string{entry[:idx], entry[idx+1:]}
	}
	return [2]string{entry, "*"}
}

// codename ekstrak codename dari PRETTY_NAME (misal "Ubuntu 22.04.3 LTS" → tidak bisa,
// tapi "Ubuntu Jammy Jellyfish" → "jammy"). Simplifikasi: ambil kata kedua lowercase.
func codename(prettyName string) string {
	words := splitWords(prettyName)
	if len(words) >= 2 {
		return toLower(words[1])
	}
	return ""
}

func splitWords(s string) []string {
	var words []string
	var current []byte
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = current[:0]
			}
		} else {
			current = append(current, s[i])
		}
	}
	if len(current) > 0 {
		words = append(words, string(current))
	}
	return words
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func hasBuildFunctions(s *resolver.UniScript) bool {
	return s.BuildFn != "" || s.PrepareFn != "" || s.PackageFn != ""
}

func must(err error, context string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "[unipack] error %s: %v\n", context, err)
		os.Exit(1)
	}
}

func die(msg string) {
	fmt.Fprintf(os.Stderr, "[unipack] %s\n", msg)
	os.Exit(1)
}

func printHelp() {
	fmt.Printf(`unipack %s — Universal Linux Package Manager Builder
Gabungan nFPM + FPM + Pacstall + Alien

Penggunaan:
  unipack setup [--dry-run]
  unipack build <script.uniscript> [--format FORMAT] [--output DIR]
  unipack convert <package> --to FORMAT [--output DIR]
  unipack init [--name NAME] [--output FILE]
  unipack detect
  unipack version

Subcommands:
  setup     Install otomatis nFPM, FPM, Alien, Ruby, dan dependensi sistem
  build     Build paket dari UniScript (source atau binary)
  convert   Konversi paket antar format (menggunakan Alien)
  init      Generate template UniScript kosong
  detect    Tampilkan info distro yang terdeteksi
  version   Tampilkan versi unipack

Format yang didukung:
  deb        Debian/Ubuntu        (nFPM utama, FPM fallback)
  rpm        Fedora/RHEL/openSUSE (nFPM utama, FPM fallback)
  apk        Alpine Linux         (nFPM)
  archlinux  Arch Linux           (nFPM)
  ipk        OpenWrt              (nFPM)
  tgz        Slackware            (FPM)
  solaris    Solaris/OpenIndiana  (FPM)
  freebsd    FreeBSD              (FPM)

Sumber tools:
  nFPM:     https://nfpm.goreleaser.com
  FPM:      https://github.com/jordansissel/fpm
  Pacstall: https://pacstall.dev
  Alien:    https://github.com/mildred/alien
`, version)
}

// uniscriptTemplate menghasilkan template UniScript dengan pkgname.
func uniscriptTemplate(name string) string {
	return fmt.Sprintf(`# UniScript — Universal Package Build Script
# Gabungan format Pacscript (Pacstall) + nFPM
# Dokumentasi: packagemanager/.opencode/PLAN.md

pkgname="%s"
pkgver="1.0.0"
pkgrel="1"
epoch=""
pkgdesc="Deskripsi singkat paket"
url="https://example.com"
bugs="https://example.com/issues"
license=("MIT")
maintainer=("Nama <email@example.com>")
arch=("amd64" "arm64")

# Source
source=(
  "https://github.com/example/%s/archive/refs/tags/${pkgver}.tar.gz"
)
sha256sums=("SKIP")

# Dependensi (nama generik — akan di-resolve ke nama per distro)
depends=("openssl" "zlib")
makedepends=("gcc" "make" "cmake")
checkdepends=()
optdepends=()

# Relasi
conflicts=()
replaces=()
provides=("%s")

# Kompatibilitas distro
compatible=()
incompatible=()

# File yang di-install (nFPM contents style)
# Didefinisikan di section contents: di bawah atau otomatis dari package()
# contents:
#   - src: ./build/bin/%s
#     dst: /usr/bin/%s

# Lifecycle scripts (opsional)
# scripts:
#   preinstall: ./scripts/preinstall.sh
#   postinstall: ./scripts/postinstall.sh
#   preremove: ./scripts/preremove.sh
#   postremove: ./scripts/postremove.sh

prepare() {
  cd "${pkgname}-${pkgver}"
  # ./autogen.sh
  # ./configure --prefix=/usr
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
  # atau untuk binary pre-built:
  # install -Dm755 "%s" -t "${pkgdir}/usr/bin"
}
`, name, name, name, name, name, name)
}
