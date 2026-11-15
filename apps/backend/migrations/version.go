package migrations

import (
	"embed"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// files embeds the migration names into the backend binary. Readiness can
// therefore determine the schema head even when the container has no source
// tree or .sql files beside the executable.
//
//go:embed *.sql
var files embed.FS

func Versions() []int64 {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic("read embedded migrations: " + err.Error())
	}
	versions := make([]int64, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err == nil {
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		panic("no embedded Goose migrations")
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions
}

func LatestVersion() int64 {
	versions := Versions()
	return versions[len(versions)-1]
}
