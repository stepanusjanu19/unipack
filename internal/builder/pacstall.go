// Package builder: pacstall.go — eksekusi build pipeline dari UniScript.
// Menjalankan prepare(), build(), check(), package() sebagai Bash subprocess.
package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unipack/unipack/internal/resolver"
)

// PacstallBuilder menjalankan build pipeline source-based.
type PacstallBuilder struct {
	// WorkDir direktori kerja sementara untuk build.
	WorkDir string
}

// NewPacstall membuat PacstallBuilder dengan workdir di tmpdir.
func NewPacstall() (*PacstallBuilder, error) {
	workDir, err := os.MkdirTemp("", "unipack-build-*")
	if err != nil {
		return nil, fmt.Errorf("buat build workdir: %w", err)
	}
	return &PacstallBuilder{WorkDir: workDir}, nil
}

// PipelineResult hasil dari pipeline build.
type PipelineResult struct {
	PkgDir string // path ke DESTDIR hasil package()
}

// Run menjalankan full pipeline: download source → prepare → build → check → package.
func (p *PacstallBuilder) Run(s *resolver.UniScript) (*PipelineResult, error) {
	pkgDir := filepath.Join(p.WorkDir, "pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return nil, fmt.Errorf("buat pkgdir: %w", err)
	}

	srcDir := filepath.Join(p.WorkDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return nil, fmt.Errorf("buat srcdir: %w", err)
	}

	// Download & extract sources
	if err := p.downloadSources(s, srcDir); err != nil {
		return nil, fmt.Errorf("download sources: %w", err)
	}

	// Jalankan tiap phase sebagai Bash script
	phases := []struct {
		name string
		body string
	}{
		{"prepare", s.PrepareFn},
		{"build", s.BuildFn},
		{"check", s.CheckFn},
		{"package", s.PackageFn},
	}

	for _, phase := range phases {
		if strings.TrimSpace(phase.body) == "" {
			continue
		}
		if err := p.runPhase(s, phase.name, phase.body, srcDir, pkgDir); err != nil {
			return nil, fmt.Errorf("phase %s gagal: %w", phase.name, err)
		}
	}

	return &PipelineResult{PkgDir: pkgDir}, nil
}

// downloadSources mengunduh semua URL di source[].
// Gunakan curl/wget dengan SKIP checksum jika sum == "SKIP".
func (p *PacstallBuilder) downloadSources(s *resolver.UniScript, srcDir string) error {
	sha256sums := s.Sums["sha256"]

	for i, src := range s.Source {
		// Ganti ${pkgver} dll dalam URL
		src = expandVars(src, s)

		// Tentukan nama file output
		outFile := filepath.Join(srcDir, filepath.Base(src))

		fmt.Printf("[unipack] unduh: %s\n", src)
		cmd := exec.Command("curl", "-fsSL", "-o", outFile, src)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// fallback ke wget
			cmd = exec.Command("wget", "-q", "-O", outFile, src)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("gagal unduh %s: %w", src, err)
			}
		}

		// Verifikasi checksum jika tersedia dan bukan SKIP
		if i < len(sha256sums) && sha256sums[i] != "SKIP" {
			if err := verifySHA256(outFile, sha256sums[i]); err != nil {
				return err
			}
		}

		// Extract jika bukan di noextract
		if !contains(s.NoExtract, filepath.Base(src)) {
			if err := extractArchive(outFile, srcDir); err != nil {
				// Bukan archive — tidak masalah, file tetap tersedia
				fmt.Printf("[unipack] skip extract %s (bukan archive)\n", outFile)
			}
		}
	}
	return nil
}

// runPhase menjalankan satu fungsi Bash (prepare/build/check/package).
func (p *PacstallBuilder) runPhase(s *resolver.UniScript, phase, body, srcDir, pkgDir string) error {
	fmt.Printf("[unipack] phase: %s\n", phase)

	// Buat script Bash sementara
	script := buildBashScript(s, phase, body, srcDir, pkgDir)
	scriptPath := filepath.Join(p.WorkDir, phase+".sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("tulis script: %w", err)
	}

	cmd := exec.Command("bash", scriptPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = srcDir
	return cmd.Run()
}

// buildBashScript menghasilkan script Bash yang wrap fungsi phase.
func buildBashScript(s *resolver.UniScript, phase, body, srcDir, pkgDir string) string {
	ncpu, _ := exec.Command("nproc").Output()
	ncpuStr := strings.TrimSpace(string(ncpu))
	if ncpuStr == "" {
		ncpuStr = "1"
	}

	return fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail

# Variables dari UniScript
pkgname=%q
pkgver=%q
pkgrel=%q
srcdir=%q
pkgdir=%q
NCPU=%s

# Phase function
%s() {
%s
}

# Run
%s
`, s.PkgName, s.PkgVer, s.PkgRel, srcDir, pkgDir, ncpuStr, phase, body, phase)
}

// expandVars mengganti ${pkgver}, ${pkgname} dll dalam string.
func expandVars(s string, script *resolver.UniScript) string {
	r := strings.NewReplacer(
		"${pkgver}", script.PkgVer,
		"${pkgname}", script.PkgName,
		"${pkgrel}", script.PkgRel,
		"$pkgver", script.PkgVer,
		"$pkgname", script.PkgName,
	)
	return r.Replace(s)
}

// verifySHA256 verifikasi checksum file.
func verifySHA256(path, expected string) error {
	out, err := exec.Command("sha256sum", path).Output()
	if err != nil {
		return fmt.Errorf("sha256sum gagal: %w", err)
	}
	got := strings.Fields(string(out))[0]
	if got != expected {
		return fmt.Errorf("checksum mismatch %s: got %s want %s", filepath.Base(path), got, expected)
	}
	return nil
}

// extractArchive ekstrak file archive ke direktori tujuan.
func extractArchive(path, destDir string) error {
	lower := strings.ToLower(path)
	var cmd *exec.Cmd
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		cmd = exec.Command("tar", "-xzf", path, "-C", destDir)
	case strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2"):
		cmd = exec.Command("tar", "-xjf", path, "-C", destDir)
	case strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz"):
		cmd = exec.Command("tar", "-xJf", path, "-C", destDir)
	case strings.HasSuffix(lower, ".tar.zst"):
		cmd = exec.Command("tar", "--zstd", "-xf", path, "-C", destDir)
	case strings.HasSuffix(lower, ".zip"):
		cmd = exec.Command("unzip", "-q", path, "-d", destDir)
	default:
		return fmt.Errorf("format archive tidak dikenal: %s", filepath.Base(path))
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Cleanup menghapus workdir.
func (p *PacstallBuilder) Cleanup() {
	os.RemoveAll(p.WorkDir)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
