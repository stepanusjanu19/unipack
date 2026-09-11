// Package resolver: depmap.go — resolve nama dependensi generik ke nama per distro.
package resolver

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed data/depmap.json
var depmapRaw []byte

// depMap: map[genericName]map[distroID]packageName
var depMap map[string]map[string]string

func init() {
	if err := json.Unmarshal(depmapRaw, &depMap); err != nil {
		panic(fmt.Sprintf("depmap.json invalid: %v", err))
	}
}

// Resolve mengonversi slice dependensi generik ke nama spesifik distro.
// Jika nama tidak ada di depmap, dikembalikan apa adanya (passthrough).
// Jika nama ditemukan tapi kosong (misal systemd di alpine), di-skip.
func Resolve(deps []string, distroID string) []string {
	id := strings.ToLower(distroID)
	result := make([]string, 0, len(deps))
	for _, dep := range deps {
		// Ekstrak versi constraint jika ada: "gcc>=10" → generic="gcc", constraint=">=10"
		generic, constraint := splitConstraint(dep)
		if mapping, ok := depMap[generic]; ok {
			if native, ok := mapping[id]; ok {
				if native == "" {
					// kosong = tidak tersedia di distro ini (misal systemd di alpine)
					continue
				}
				result = append(result, native+constraint)
				continue
			}
		}
		// passthrough: tidak ada di depmap, pakai nama asli
		result = append(result, dep)
	}
	return result
}

// splitConstraint memisahkan nama dep dari version constraint.
// "openssl>=1.1" → ("openssl", ">=1.1")
// "gcc" → ("gcc", "")
func splitConstraint(dep string) (name, constraint string) {
	for _, op := range []string{">=", "<=", "!=", "==", ">", "<", "="} {
		if idx := strings.Index(dep, op); idx > 0 {
			return dep[:idx], dep[idx:]
		}
	}
	return dep, ""
}

// Available melaporkan apakah suatu package tersedia di distro (tidak kosong di depmap).
func Available(generic, distroID string) bool {
	if mapping, ok := depMap[generic]; ok {
		native := mapping[strings.ToLower(distroID)]
		return native != ""
	}
	return true // tidak ada di depmap = assume tersedia
}
