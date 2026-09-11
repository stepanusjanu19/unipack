// Package setup mendeteksi dan menginstal otomatis tools yang dibutuhkan unipack:
// nFPM, FPM, Alien, Ruby, dan dependensi sistem (curl, tar, make, gcc).
package setup

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/stepanusjanu19/unipack/internal/detector"
)

// NFPMVersion adalah versi nFPM yang akan diunduh.
const NFPMVersion = "v2.47.0"

// ToolDir adalah direktori penyimpanan tools yang diunduh (nFPM binary).
// Ditaruh di ~/.local/share/unipack/bin agar tidak menyentuh /usr/bin.
func ToolDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "share", "unipack", "bin")
	return dir, os.MkdirAll(dir, 0755)
}

// Check memeriksa apakah suatu binary tersedia di PATH atau di ToolDir.
func Check(name string) bool {
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	dir, err := ToolDir()
	if err != nil {
		return false
	}
	full := filepath.Join(dir, name)
	if _, err := os.Stat(full); err == nil {
		return true
	}
	return false
}

// EnsurePATH memastikan ToolDir ada di PATH (untuk sesi shell saat ini).
func EnsurePATH() error {
	dir, err := ToolDir()
	if err != nil {
		return err
	}
	current := os.Getenv("PATH")
	if !strings.Contains(current, dir) {
		return os.Setenv("PATH", dir+":"+current)
	}
	return nil
}

// Result menyimpan hasil setup per tool.
type Result struct {
	Tool    string
	Installed bool
	Skipped  bool
	Error   error
}

// Run menjalankan full setup: cek & install semua tools yang hilang.
// dryRun=true hanya melaporkan apa yang akan dilakukan tanpa eksekusi.
func Run(dryRun bool) []Result {
	var results []Result

	distro, err := detector.Detect()
	if err != nil {
		results = append(results, Result{Tool: "distro-detect", Error: err})
		return results
	}

	// 1. System packages: curl, tar, make, gcc, ruby, ruby-dev
	results = append(results, ensureSystemPackages(distro, dryRun)...)

	// 2. nFPM (download binary dari GitHub)
	results = append(results, ensureNFPM(dryRun))

	// 3. FPM (gem install)
	results = append(results, ensureFPM(dryRun))

	// 4. Alien (system package)
	results = append(results, ensureAlien(distro, dryRun))

	return results
}

// ensureSystemPackages menginstall paket sistem dasar via package manager OS.
func ensureSystemPackages(distro *detector.Distro, dryRun bool) []Result {
	var results []Result
	pkgs := []string{"curl", "tar", "make", "gcc", "ruby"}
	pkgDevel := map[string]string{"ruby": "ruby-dev"} // devel packages

	missing := []string{}
	missingDevel := []string{}
	for _, pkg := range pkgs {
		if !Check(pkg) && !isSystemPkgInstalled(distro, pkg) {
			missing = append(missing, pkg)
			if dev, ok := pkgDevel[pkg]; ok {
				missingDevel = append(missingDevel, dev)
			}
		} else {
			results = append(results, Result{Tool: pkg, Skipped: true})
		}
	}

	if len(missing) == 0 {
		return results
	}

	all := append(missing, missingDevel...)
	if dryRun {
		for _, p := range all {
			results = append(results, Result{Tool: p, Error: fmt.Errorf("DRY RUN: would install via %s", distro.PackageCmd)})
		}
		return results
	}

	if err := installViaPackageManager(distro, all); err != nil {
		for _, p := range all {
			results = append(results, Result{Tool: p, Error: err})
		}
	} else {
		for _, p := range all {
			results = append(results, Result{Tool: p, Installed: true})
		}
	}
	return results
}

// ensureNFPM mengunduh binary nFPM dari GitHub releases.
func ensureNFPM(dryRun bool) Result {
	if Check("nfpm") {
		return Result{Tool: "nfpm", Skipped: true}
	}

	if dryRun {
		return Result{Tool: "nfpm", Error: fmt.Errorf("DRY RUN: would download nFPM %s from GitHub", NFPMVersion)}
	}

	dir, err := ToolDir()
	if err != nil {
		return Result{Tool: "nfpm", Error: err}
	}

	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	}

	// NFPMVersion dengan "v" prefix untuk tag, tanpa "v" untuk nama file
	verNum := strings.TrimPrefix(NFPMVersion, "v")
	url := fmt.Sprintf("https://github.com/goreleaser/nfpm/releases/download/%s/nfpm_%s_Linux_%s.tar.gz",
		NFPMVersion, verNum, arch)

	fmt.Printf("[setup] unduh nFPM dari %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return Result{Tool: "nfpm", Error: fmt.Errorf("download nfpm: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Result{Tool: "nfpm", Error: fmt.Errorf("download nfpm: HTTP %d", resp.StatusCode)}
	}

	// Simpan ke file sementara lalu ekstrak
	tmpTar := filepath.Join(dir, "nfpm.tar.gz")
	out, err := os.Create(tmpTar)
	if err != nil {
		return Result{Tool: "nfpm", Error: err}
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return Result{Tool: "nfpm", Error: err}
	}
	out.Close()
	defer os.Remove(tmpTar)

	// Ekstrak dengan tar
	cmd := exec.Command("tar", "-xzf", tmpTar, "-C", dir, "nfpm")
	if err := cmd.Run(); err != nil {
		return Result{Tool: "nfpm", Error: fmt.Errorf("extract nfpm: %w", err)}
	}

	// chmod +x
	nfpmPath := filepath.Join(dir, "nfpm")
	if err := os.Chmod(nfpmPath, 0755); err != nil {
		return Result{Tool: "nfpm", Error: err}
	}

	// Coba symlink ke /usr/local/bin agar tersedia di PATH secara permanen
	os.Symlink(nfpmPath, "/usr/local/bin/nfpm")

	return Result{Tool: "nfpm", Installed: true}
}

// ensureFPM menginstall FPM via RubyGems.
func ensureFPM(dryRun bool) Result {
	if Check("fpm") {
		return Result{Tool: "fpm", Skipped: true}
	}
	if !Check("ruby") {
		return Result{Tool: "fpm", Error: fmt.Errorf("ruby belum terinstall — jalankan setup lagi setelah ruby terpasang")}
	}

	if dryRun {
		return Result{Tool: "fpm", Error: fmt.Errorf("DRY RUN: would run `gem install fpm`")}
	}

	cmd := exec.Command("gem", "install", "--no-document", "fpm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return Result{Tool: "fpm", Error: fmt.Errorf("gem install fpm: %w", err)}
	}
	return Result{Tool: "fpm", Installed: true}
}

// ensureAlien menginstall Alien via package manager OS.
func ensureAlien(distro *detector.Distro, dryRun bool) Result {
	if Check("alien") {
		return Result{Tool: "alien", Skipped: true}
	}

	if dryRun {
		return Result{Tool: "alien", Error: fmt.Errorf("DRY RUN: would install alien via %s", distro.PackageCmd)}
	}

	if err := installViaPackageManager(distro, []string{"alien"}); err != nil {
		return Result{Tool: "alien", Error: err}
	}
	return Result{Tool: "alien", Installed: true}
}

// installViaPackageManager menjalankan apt/dnf/pacman/apk install.
func installViaPackageManager(distro *detector.Distro, pkgs []string) error {
	if len(pkgs) == 0 {
		return nil
	}

	var cmd *exec.Cmd
	switch distro.PackageCmd {
	case "apt":
		// Update package index first
		exec.Command("sudo", "apt-get", "update").Run()
		args := append([]string{"apt-get", "install", "-y"}, pkgs...)
		cmd = exec.Command("sudo", args...)
	case "dnf":
		args := append([]string{"dnf", "install", "-y"}, pkgs...)
		cmd = exec.Command("sudo", args...)
	case "yum":
		args := append([]string{"yum", "install", "-y"}, pkgs...)
		cmd = exec.Command("sudo", args...)
	case "pacman":
		args := append([]string{"pacman", "-Sy", "--noconfirm"}, pkgs...)
		cmd = exec.Command("sudo", args...)
	case "apk":
		args := append([]string{"apk", "add"}, pkgs...)
		cmd = exec.Command("sudo", args...)
	case "zypper":
		args := append([]string{"zypper", "install", "-y"}, pkgs...)
		cmd = exec.Command("sudo", args...)
	default:
		return fmt.Errorf("package manager tidak dikenal: %s (install manual: %s)",
			distro.PackageCmd, strings.Join(pkgs, " "))
	}

	fmt.Printf("[setup] %s %s\n", cmd.Path, strings.Join(cmd.Args[1:], " "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isSystemPkgInstalled cek apakah paket sistem sudah terinstall.
func isSystemPkgInstalled(distro *detector.Distro, pkg string) bool {
	var cmd *exec.Cmd
	switch distro.PackageCmd {
	case "apt":
		cmd = exec.Command("dpkg", "-s", pkg)
	case "dnf", "yum", "zypper":
		cmd = exec.Command("rpm", "-q", pkg)
	case "pacman":
		cmd = exec.Command("pacman", "-Q", pkg)
	case "apk":
		cmd = exec.Command("apk", "info", "-e", pkg)
	default:
		return false
	}
	return cmd.Run() == nil
}
