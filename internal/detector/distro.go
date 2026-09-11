// Package detector mendeteksi distro Linux dari /etc/os-release.
package detector

import (
	"bufio"
	"os"
	"strings"
)

// Distro menyimpan informasi OS yang terdeteksi.
type Distro struct {
	ID         string // contoh: ubuntu, fedora, arch
	IDLike     string // contoh: debian, rhel
	VersionID  string // contoh: 22.04
	PrettyName string // contoh: Ubuntu 22.04.3 LTS
	PackageFmt string // deb, rpm, apk, pacman, tgz
	PackageCmd string // apt, dnf, pacman, apk, slackpkg
}

// Detect membaca /etc/os-release dan mengembalikan Distro.
// Fallback ke "unknown" jika file tidak ada.
func Detect() (*Distro, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return fallback(), nil
	}
	defer f.Close()

	fields := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		fields[key] = val
	}

	d := &Distro{
		ID:         strings.ToLower(fields["ID"]),
		IDLike:     strings.ToLower(fields["ID_LIKE"]),
		VersionID:  fields["VERSION_ID"],
		PrettyName: fields["PRETTY_NAME"],
	}
	d.PackageFmt, d.PackageCmd = resolvePackageSystem(d.ID, d.IDLike)
	return d, nil
}

// FamilyID mengembalikan ID canonical untuk keperluan depmap lookup.
// Prioritas: ID langsung → ID_LIKE → "unknown".
func (d *Distro) FamilyID() string {
	known := map[string]string{
		"ubuntu": "ubuntu", "debian": "debian", "linuxmint": "debian",
		"pop": "ubuntu", "elementary": "ubuntu", "kali": "debian",
		"fedora": "fedora", "rhel": "rhel", "centos": "centos",
		"rocky": "rhel", "almalinux": "rhel", "ol": "rhel",
		"arch": "arch", "manjaro": "arch", "endeavouros": "arch",
		"alpine": "alpine",
		"opensuse-leap": "opensuse", "opensuse-tumbleweed": "opensuse", "opensuse": "opensuse",
		"slackware": "slackware",
	}
	if v, ok := known[d.ID]; ok {
		return v
	}
	// cek ID_LIKE
	for _, like := range strings.Fields(d.IDLike) {
		if v, ok := known[like]; ok {
			return v
		}
	}
	return "unknown"
}

func resolvePackageSystem(id, idLike string) (format, cmd string) {
	debianFamily := []string{"debian", "ubuntu", "linuxmint", "pop", "elementary", "kali", "raspbian"}
	rpmFamily := []string{"fedora", "rhel", "centos", "rocky", "almalinux", "ol", "opensuse", "opensuse-leap", "opensuse-tumbleweed", "suse"}
	archFamily := []string{"arch", "manjaro", "endeavouros", "artix", "garuda"}

	check := func(list []string, target string) bool {
		for _, v := range list {
			if v == target {
				return true
			}
		}
		return false
	}

	all := append([]string{id}, strings.Fields(idLike)...)
	for _, s := range all {
		switch {
		case check(debianFamily, s):
			return "deb", "apt"
		case check(rpmFamily, s):
			if strings.Contains(s, "opensuse") || strings.Contains(s, "suse") {
				return "rpm", "zypper"
			}
			if s == "fedora" {
				return "rpm", "dnf"
			}
			return "rpm", "dnf"
		case check(archFamily, s):
			return "pacman", "pacman"
		case s == "alpine":
			return "apk", "apk"
		case s == "slackware":
			return "tgz", "slackpkg"
		}
	}
	return "unknown", "unknown"
}

func fallback() *Distro {
	return &Distro{
		ID:         "unknown",
		PackageFmt: "unknown",
		PackageCmd: "unknown",
	}
}
