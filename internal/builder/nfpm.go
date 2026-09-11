// Package builder: nfpm.go — wrapper nFPM untuk build paket native.
// nFPM dipanggil sebagai subprocess karena user mungkin pakai nFPM binary
// yang sudah terinstall, bukan Go library (hindari dependency tree membengkak).
package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/stepanusjanu19/unipack/internal/resolver"
)

// NFPMBuilder membungkus nFPM CLI.
type NFPMBuilder struct {
	// BinPath path ke binary nFPM. Default: "nfpm" (dari PATH).
	BinPath string
}

// NewNFPM membuat NFPMBuilder. Cek apakah nFPM tersedia di PATH.
func NewNFPM() (*NFPMBuilder, error) {
	bin, err := exec.LookPath("nfpm")
	if err != nil {
		return nil, fmt.Errorf("nfpm tidak ditemukan di PATH: %w", err)
	}
	return &NFPMBuilder{BinPath: bin}, nil
}

// BuildOptions adalah input untuk NFPMBuilder.Build.
type BuildOptions struct {
	Script    *resolver.UniScript
	DistroID  string        // untuk resolve dependensi
	Format    string        // "deb", "rpm", "apk", "archlinux", "ipk"
	OutputDir string        // direktori output paket
	PkgDir    string        // direktori hasil package() function (DESTDIR)
}

// Build menghasilkan paket dengan nFPM.
// Alur: generate nfpm.yaml sementara → jalankan `nfpm pkg`.
func (n *NFPMBuilder) Build(opts BuildOptions) (string, error) {
	if opts.Format == "" {
		return "", fmt.Errorf("format tidak boleh kosong")
	}

	// Tulis nfpm.yaml ke tempdir
	tmpDir, err := os.MkdirTemp("", "unipack-nfpm-*")
	if err != nil {
		return "", fmt.Errorf("buat tempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "nfpm.yaml")
	if err := writeNFPMConfig(cfgPath, opts); err != nil {
		return "", fmt.Errorf("tulis nfpm.yaml: %w", err)
	}

	// Jalankan nFPM
	cmd := exec.Command(n.BinPath, "pkg",
		"--packager", opts.Format,
		"--config", cfgPath,
		"--target", opts.OutputDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nfpm pkg gagal: %w", err)
	}

	// Cari file output
	out, err := findOutputFile(opts.OutputDir, opts.Format)
	if err != nil {
		return "", err
	}
	return out, nil
}

// nfpmConfigTmpl adalah template nfpm.yaml.
const nfpmConfigTmpl = `name: {{ .Name }}
version: "{{ .Version }}"
release: "{{ .Release }}"
{{- if .Epoch }}
epoch: {{ .Epoch }}
{{- end }}
arch: {{ .Arch }}
platform: linux
maintainer: "{{ .Maintainer }}"
description: |
  {{ .Description }}
homepage: "{{ .Homepage }}"
license: {{ .License }}

contents:
{{- range .Contents }}
  - src: {{ .Src }}
    dst: {{ .Dst }}
    {{- if .Type }}
    type: {{ .Type }}
    {{- end }}
{{- end }}

{{- if or .PreInstall .PostInstall .PreRemove .PostRemove }}
scripts:
  {{- if .PreInstall }}
  preinstall: {{ .PreInstall }}
  {{- end }}
  {{- if .PostInstall }}
  postinstall: {{ .PostInstall }}
  {{- end }}
  {{- if .PreRemove }}
  preremove: {{ .PreRemove }}
  {{- end }}
  {{- if .PostRemove }}
  postremove: {{ .PostRemove }}
  {{- end }}
{{- end }}

{{- if .Depends }}
depends:
{{- range .Depends }}
  - {{ . }}
{{- end }}
{{- end }}

{{- if .Overrides }}
overrides:
{{ .Overrides }}
{{- end }}
`

type nfpmConfigData struct {
	Name        string
	Version     string
	Release     string
	Epoch       string
	Arch        string
	Maintainer  string
	Description string
	Homepage    string
	License     string
	Contents    []resolver.Content
	PreInstall  string
	PostInstall string
	PreRemove   string
	PostRemove  string
	Depends     []string
	Overrides   string // raw YAML block per-format overrides
}

func writeNFPMConfig(path string, opts BuildOptions) error {
	s := opts.Script

	// Resolve arch: ambil pertama atau "amd64"
	arch := "amd64"
	if len(s.Arch) > 0 && s.Arch[0] != "any" {
		arch = s.Arch[0]
	}

	maintainer := ""
	if len(s.Maintainer) > 0 {
		maintainer = s.Maintainer[0]
	}

	license := ""
	if len(s.License) > 0 {
		license = s.License[0]
	}

	version := s.PkgVer
	release := s.PkgRel
	if release == "" {
		release = "1"
	}

	// Resolve dependensi ke nama native distro
	depends := resolver.Resolve(s.Depends, opts.DistroID)

	// Build overrides YAML jika ada per-format override di script
	overridesYAML := buildOverridesYAML(s, opts.DistroID)

	data := nfpmConfigData{
		Name:        s.PkgName,
		Version:     version,
		Release:     release,
		Epoch:       s.Epoch,
		Arch:        arch,
		Maintainer:  maintainer,
		Description: s.PkgDesc,
		Homepage:    s.URL,
		License:     license,
		Contents:    s.Contents,
		PreInstall:  s.Scripts.PreInstall,
		PostInstall: s.Scripts.PostInstall,
		PreRemove:   s.Scripts.PreRemove,
		PostRemove:  s.Scripts.PostRemove,
		Depends:     depends,
		Overrides:   overridesYAML,
	}

	tmpl, err := template.New("nfpm").Parse(nfpmConfigTmpl)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// buildOverridesYAML membuat blok overrides YAML dari UniScript.Overrides.
func buildOverridesYAML(s *resolver.UniScript, distroID string) string {
	if len(s.Overrides) == 0 {
		return ""
	}
	var sb strings.Builder
	for format, override := range s.Overrides {
		sb.WriteString(fmt.Sprintf("  %s:\n", format))
		if len(override.Depends) > 0 {
			sb.WriteString("    depends:\n")
			for _, d := range override.Depends {
				sb.WriteString(fmt.Sprintf("      - %s\n", d))
			}
		}
	}
	return sb.String()
}

// findOutputFile mencari file paket yang baru dibuat di outputDir.
func findOutputFile(dir, format string) (string, error) {
	ext := map[string]string{
		"deb":       ".deb",
		"rpm":       ".rpm",
		"apk":       ".apk",
		"archlinux": ".pkg.tar.zst",
		"ipk":       ".ipk",
	}
	suffix, ok := ext[format]
	if !ok {
		suffix = "." + format
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("baca outputDir: %w", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("file output %s tidak ditemukan di %s", suffix, dir)
}
