// Package converter: alien.go — wrapper Alien untuk konversi format paket.
// Alien dipakai sebagai last resort: tidak ada source, hanya paket biner format lain.
package converter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AlienConverter membungkus alien CLI (Perl).
type AlienConverter struct {
	BinPath string
}

// NewAlien membuat AlienConverter. Cek apakah alien tersedia.
func NewAlien() (*AlienConverter, error) {
	bin, err := exec.LookPath("alien")
	if err != nil {
		return nil, fmt.Errorf("alien tidak ditemukan di PATH (install: apt install alien): %w", err)
	}
	return &AlienConverter{BinPath: bin}, nil
}

// ConvertOptions adalah opsi konversi paket.
type ConvertOptions struct {
	InputFile string // path ke paket sumber (.rpm/.deb/.tgz/.slp)
	OutputDir string // direktori untuk menyimpan hasil konversi
	ToFormat  string // "deb", "rpm", "tgz", "slp" (Stampede)
	Scripts   bool   // true: coba konversi lifecycle scripts
	Verbose   bool
}

// SupportedInput format yang alien bisa baca.
var SupportedInput = []string{".rpm", ".deb", ".tgz", ".slp", ".pkg"}

// SupportedOutput format yang alien bisa hasilkan.
var SupportedOutput = []string{"deb", "rpm", "tgz", "slp"}

// Convert menjalankan alien untuk mengubah format paket.
//
// PERINGATAN: Alien rentan gagal pada paket dengan:
// - Hardcoded path ke direktori distro asal
// - Post-install script yang bergantung pada systemd unit distro asal
// - Library dependency name yang tidak bisa di-resolve otomatis
//
// Selalu verifikasi hasil konversi sebelum diinstall.
func (a *AlienConverter) Convert(opts ConvertOptions) (string, error) {
	if opts.InputFile == "" {
		return "", fmt.Errorf("InputFile tidak boleh kosong")
	}
	if opts.ToFormat == "" {
		return "", fmt.Errorf("ToFormat tidak boleh kosong")
	}

	// Validasi format output
	if !isSupportedOutput(opts.ToFormat) {
		return "", fmt.Errorf("format output %q tidak didukung alien. Pilihan: %s",
			opts.ToFormat, strings.Join(SupportedOutput, ", "))
	}

	// Alien harus dijalankan dari outputDir (file dihasilkan di CWD)
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return "", fmt.Errorf("buat outputDir: %w", err)
	}

	args := a.buildArgs(opts)

	fmt.Printf("[unipack] alien: konversi %s ke %s\n", filepath.Base(opts.InputFile), opts.ToFormat)

	cmd := exec.Command(a.BinPath, args...)
	cmd.Dir = opts.OutputDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("alien gagal: %w\nCatatan: verifikasi manual hasil konversi sebelum install", err)
	}

	// Temukan file output
	ext := "." + opts.ToFormat
	if opts.ToFormat == "rpm" {
		ext = ".rpm"
	}
	out, err := findAlienOutput(opts.OutputDir, ext)
	if err != nil {
		return "", err
	}

	fmt.Printf("[unipack] hasil konversi: %s\n", out)
	fmt.Println("[unipack] PERINGATAN: Verifikasi dependency alien result sebelum install.")
	return out, nil
}

// buildArgs menyusun argumen CLI untuk alien.
func (a *AlienConverter) buildArgs(opts ConvertOptions) []string {
	// Flag format output
	formatFlag := map[string]string{
		"deb": "--to-deb",
		"rpm": "--to-rpm",
		"tgz": "--to-tgz",
		"slp": "--to-slp",
	}

	args := []string{
		formatFlag[opts.ToFormat],
		"--keep-version",
	}

	if opts.Scripts {
		args = append(args, "--scripts")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}

	// File input — alien butuh path absolut
	absInput, err := filepath.Abs(opts.InputFile)
	if err != nil {
		absInput = opts.InputFile
	}
	args = append(args, absInput)

	return args
}

// DetectInputFormat mendeteksi format paket dari ekstensi file.
func DetectInputFormat(path string) (string, error) {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".rpm"):
		return "rpm", nil
	case strings.HasSuffix(lower, ".deb"):
		return "deb", nil
	case strings.HasSuffix(lower, ".tgz"):
		return "tgz", nil
	case strings.HasSuffix(lower, ".slp"):
		return "slp", nil
	default:
		return "", fmt.Errorf("format tidak dikenal untuk file: %s", filepath.Base(path))
	}
}

// NeedsConversion melaporkan apakah file perlu dikonversi untuk distro target.
// targetFormat: "deb", "rpm", dll.
func NeedsConversion(inputFile, targetFormat string) bool {
	inputFmt, err := DetectInputFormat(inputFile)
	if err != nil {
		return false
	}
	return inputFmt != targetFormat
}

func isSupportedOutput(format string) bool {
	for _, v := range SupportedOutput {
		if v == format {
			return true
		}
	}
	return false
}

func findAlienOutput(dir, ext string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("baca outputDir: %w", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ext) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("file output %s tidak ditemukan di %s", ext, dir)
}
