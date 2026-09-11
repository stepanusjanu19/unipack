// Package builder: fpm.go — wrapper FPM (Ruby) sebagai builder fallback.
// Dipanggil jika nFPM tidak tersedia atau format target tidak didukung nFPM.
// Format unik yang hanya FPM support: pacman (lama), solaris, freebsd, p5p, osxpkg.
package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unipack/unipack/internal/resolver"
)

// FPMBuilder membungkus FPM CLI.
type FPMBuilder struct {
	BinPath string
}

// NewFPM membuat FPMBuilder. Cek apakah fpm tersedia di PATH.
func NewFPM() (*FPMBuilder, error) {
	bin, err := exec.LookPath("fpm")
	if err != nil {
		return nil, fmt.Errorf("fpm tidak ditemukan di PATH (install: gem install fpm): %w", err)
	}
	return &FPMBuilder{BinPath: bin}, nil
}

// FPMSupports melaporkan apakah format target didukung FPM tapi tidak nFPM.
// nFPM support: deb, rpm, apk, archlinux, ipk.
// FPM-only: pacman (lama), solaris, freebsd, p5p, osxpkg, sh, tar.
func FPMSupports(format string) bool {
	fpmOnly := map[string]bool{
		"solaris": true,
		"freebsd": true,
		"p5p":     true,
		"osxpkg":  true,
		"sh":      true,
	}
	return fpmOnly[format]
}

// Build menghasilkan paket dengan FPM dari direktori hasil build.
func (f *FPMBuilder) Build(opts BuildOptions) (string, error) {
	if opts.PkgDir == "" {
		return "", fmt.Errorf("PkgDir (DESTDIR hasil build) tidak boleh kosong")
	}

	s := opts.Script
	args := f.buildArgs(s, opts)

	cmd := exec.Command(f.BinPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = opts.PkgDir

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("fpm gagal: %w", err)
	}

	out, err := findOutputFile(opts.OutputDir, opts.Format)
	if err != nil {
		return "", err
	}
	return out, nil
}

// buildArgs menyusun argumen CLI untuk fpm.
func (f *FPMBuilder) buildArgs(s *resolver.UniScript, opts BuildOptions) []string {
	depends := resolver.Resolve(s.Depends, opts.DistroID)

	args := []string{
		"-s", "dir",
		"-t", opts.Format,
		"-n", s.PkgName,
		"-v", s.PkgVer,
		"--iteration", orDefault(s.PkgRel, "1"),
		"--description", s.PkgDesc,
		"-p", filepath.Join(opts.OutputDir, "NAME-VERSION_ARCH.EXTENSION"),
	}

	if s.URL != "" {
		args = append(args, "--url", s.URL)
	}
	if len(s.Maintainer) > 0 {
		args = append(args, "--maintainer", s.Maintainer[0])
	}
	if len(s.License) > 0 {
		args = append(args, "--license", s.License[0])
	}

	// Dependensi
	for _, d := range depends {
		args = append(args, "-d", d)
	}

	// Conflicts
	for _, c := range s.Conflicts {
		args = append(args, "--conflicts", c)
	}

	// Provides
	for _, p := range s.Provides {
		args = append(args, "--provides", p)
	}

	// Replaces
	for _, r := range s.Replaces {
		args = append(args, "--replaces", r)
	}

	// Lifecycle scripts
	if s.Scripts.PreInstall != "" {
		args = append(args, "--before-install", s.Scripts.PreInstall)
	}
	if s.Scripts.PostInstall != "" {
		args = append(args, "--after-install", s.Scripts.PostInstall)
	}
	if s.Scripts.PreRemove != "" {
		args = append(args, "--before-remove", s.Scripts.PreRemove)
	}
	if s.Scripts.PostRemove != "" {
		args = append(args, "--after-remove", s.Scripts.PostRemove)
	}

	// Config files
	for _, b := range s.Backup {
		args = append(args, "--config-files", b)
	}

	// Arch
	arch := "x86_64"
	if len(s.Arch) > 0 && s.Arch[0] != "any" {
		arch = mapArchFPM(s.Arch[0])
	}
	args = append(args, "--architecture", arch)

	// Source directory: semua isi PkgDir
	args = append(args, opts.PkgDir+"/=./")

	return args
}

// mapArchFPM menerjemahkan nama arsitektur ke format FPM.
func mapArchFPM(arch string) string {
	m := map[string]string{
		"amd64":   "x86_64",
		"arm64":   "aarch64",
		"arm":     "armv7l",
		"i386":    "i386",
		"ppc64le": "ppc64le",
		"s390x":   "s390x",
	}
	if v, ok := m[arch]; ok {
		return v
	}
	return arch
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
