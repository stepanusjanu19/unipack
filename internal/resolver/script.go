// Package resolver mem-parse UniScript — format script pemaketan gabungan
// yang menggabungkan field Pacscript (Bash) + nFPM (YAML).
package resolver

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Content merepresentasikan satu file yang di-install (nFPM contents style).
type Content struct {
	Src  string
	Dst  string
	Type string // "config", "config|noreplace", "ghost", "" (regular)
}

// Scripts lifecycle hooks (pre/post install/remove).
type Scripts struct {
	PreInstall  string
	PostInstall string
	PreRemove   string
	PostRemove  string
}

// Overrides per package format (seperti nFPM overrides).
type FormatOverride struct {
	Depends []string
}

// UniScript adalah representasi parsed dari file .uniscript.
type UniScript struct {
	// Metadata
	PkgName     string
	PkgBase     string
	PkgVer      string
	PkgRel      string
	Epoch       string
	PkgDesc     string
	URL         string
	Bugs        string
	License     []string
	Maintainer  []string
	Arch        []string
	Repology    []string

	// Source & checksums
	Source   []string
	Sums     map[string][]string // key: sha256/sha512/b2/md5

	// Dependencies
	Depends      []string
	MakeDepends  []string
	CheckDepends []string
	OptDepends   []string
	PacDeps      []string

	// Relasi
	Conflicts []string
	Breaks    []string
	Replaces  []string
	Provides  []string
	Gives     string

	// Kompatibilitas
	Compatible   []string
	Incompatible []string

	// Instalasi
	Contents []Content
	Scripts  Scripts
	Backup   []string
	Priority string
	Mask     []string

	// Per-format overrides
	Overrides map[string]FormatOverride

	// Raw build functions (Bash source)
	PrepareFn string
	BuildFn   string
	CheckFn   string
	PackageFn string

	// Flags
	ExternalConnection bool
	NoSubmodules       []string
	NoExtract          []string
}

// ParseFile membaca file .uniscript dan mengembalikan UniScript.
// Format: Bash-style key=value + array=() + function blocks.
func ParseFile(path string) (*UniScript, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	u := &UniScript{
		Sums:      make(map[string][]string),
		Overrides: make(map[string]FormatOverride),
	}

	lines := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])

		// skip comment & blank
		if line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}

		// detect function block
		if isFunctionStart(line) {
			name, body, next := parseFunctionBlock(lines, i)
			switch name {
			case "prepare":
				u.PrepareFn = body
			case "build":
				u.BuildFn = body
			case "check":
				u.CheckFn = body
			case "package":
				u.PackageFn = body
			}
			i = next
			continue
		}

		// key=value or key=()
		if strings.Contains(line, "=") {
			key, val, arr, isArr := parseLine(line, lines, i)
			i++ // default advance
			if isArr {
				// array spans multiple lines: advance past the closing paren
				_, _, _, _, endIdx := parseArray(lines, i-1)
				i = endIdx + 1
				assignArray(u, key, arr)
			} else {
				assignScalar(u, key, val)
			}
			continue
		}

		i++
	}

	return u, nil
}

// isFunctionStart deteksi baris seperti `build() {` atau `build () {`.
func isFunctionStart(line string) bool {
	for _, fn := range []string{"prepare()", "build()", "check()", "package()", "prepare ()", "build ()", "check ()", "package ()"} {
		if strings.HasPrefix(line, fn) {
			return true
		}
	}
	return false
}

// parseFunctionBlock mengekstrak nama fungsi dan isi body { ... }.
// Mengembalikan name, body, dan index baris setelah penutup '}'.
func parseFunctionBlock(lines []string, start int) (name, body string, next int) {
	header := strings.TrimSpace(lines[start])
	name = strings.TrimSuffix(strings.TrimSuffix(strings.Fields(header)[0], "()"), " ()")
	name = strings.TrimSpace(strings.Split(name, "(")[0])

	var sb strings.Builder
	depth := 0
	i := start
	for i < len(lines) {
		l := lines[i]
		depth += strings.Count(l, "{") - strings.Count(l, "}")
		if i > start {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}
		i++
		if depth <= 0 && i > start+1 {
			break
		}
	}
	return name, strings.TrimSpace(sb.String()), i
}

// parseLine mengurai baris key=val atau key=(arr).
// Jika array, isArr=true dan arr berisi elemen-elemennya.
func parseLine(line string, lines []string, idx int) (key, val string, arr []string, isArr bool) {
	parts := strings.SplitN(line, "=", 2)
	key = strings.TrimSpace(parts[0])
	raw := strings.TrimSpace(parts[1])

	if strings.HasPrefix(raw, "(") {
		isArr = true
		_, arr, _, _, _ = parseArray(lines, idx)
		return
	}
	// scalar: strip quotes
	val = strings.Trim(raw, `"'`)
	return
}

// parseArray mengurai array bash multi-baris yang dimulai di baris idx.
// Mengembalikan elemen array dan index baris penutup ')'.
func parseArray(lines []string, idx int) (key string, elems []string, raw string, startIdx, endIdx int) {
	startIdx = idx
	// gabungkan semua sampai ')' penutup
	var sb strings.Builder
	for i := idx; i < len(lines); i++ {
		sb.WriteString(lines[i])
		sb.WriteByte('\n')
		endIdx = i
		if strings.Contains(lines[i], ")") && i > idx {
			break
		}
		// single line: "key=(...)"
		if i == idx && strings.Count(lines[i], "(") == strings.Count(lines[i], ")") {
			break
		}
	}
	combined := sb.String()
	// strip sampai '(' pertama
	open := strings.Index(combined, "(")
	close := strings.LastIndex(combined, ")")
	if open < 0 || close < 0 {
		return
	}
	inner := combined[open+1 : close]
	// split per newline/space, strip quotes & komentar
	for _, token := range strings.Fields(inner) {
		token = strings.Trim(token, `"'`)
		if token == "" || strings.HasPrefix(token, "#") {
			continue
		}
		elems = append(elems, token)
	}
	raw = combined
	return
}

// assignScalar mengisi field scalar di UniScript.
func assignScalar(u *UniScript, key, val string) {
	switch key {
	case "pkgname":
		u.PkgName = val
	case "pkgbase":
		u.PkgBase = val
	case "pkgver":
		u.PkgVer = val
	case "pkgrel":
		u.PkgRel = val
	case "epoch":
		u.Epoch = val
	case "pkgdesc":
		u.PkgDesc = val
	case "url":
		u.URL = val
	case "bugs":
		u.Bugs = val
	case "gives":
		u.Gives = val
	case "priority":
		u.Priority = val
	case "external_connection":
		u.ExternalConnection = val == "true"
	}
}

// assignArray mengisi field array di UniScript.
func assignArray(u *UniScript, key string, arr []string) {
	switch key {
	case "pkgname":
		if len(arr) > 0 {
			u.PkgName = arr[0]
		}
	case "license":
		u.License = arr
	case "maintainer":
		u.Maintainer = arr
	case "arch":
		u.Arch = arr
	case "repology":
		u.Repology = arr
	case "source":
		u.Source = arr
	case "sha256sums":
		u.Sums["sha256"] = arr
	case "sha512sums":
		u.Sums["sha512"] = arr
	case "b2sums":
		u.Sums["b2"] = arr
	case "md5sums":
		u.Sums["md5"] = arr
	case "depends":
		u.Depends = arr
	case "makedepends":
		u.MakeDepends = arr
	case "checkdepends":
		u.CheckDepends = arr
	case "optdepends":
		u.OptDepends = arr
	case "pacdeps":
		u.PacDeps = arr
	case "conflicts":
		u.Conflicts = arr
	case "breaks":
		u.Breaks = arr
	case "replaces":
		u.Replaces = arr
	case "provides":
		u.Provides = arr
	case "compatible":
		u.Compatible = arr
	case "incompatible":
		u.Incompatible = arr
	case "backup":
		u.Backup = arr
	case "mask":
		u.Mask = arr
	case "nosubmodules":
		u.NoSubmodules = arr
	case "noextract":
		u.NoExtract = arr
	}
}
