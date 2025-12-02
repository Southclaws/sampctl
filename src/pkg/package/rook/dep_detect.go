package rook

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"sort"
	"sync"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
	"github.com/Southclaws/sampctl/src/pkg/infrastructure/versioning"
)

// IncludesToDependencies maps common include paths to known Pawn package dependency strings
var IncludesToDependencies = map[string]versioning.DependencyString{
	`YSI\\.+`:       "pawn-lang/YSI-Includes",
	`streamer`:      "samp-incognito/samp-streamer-plugin",
	`map`:           "BigETI/pawn-map",
	`list`:          "BigETI/pawn-list",
	`DialogCenter`:  "Ino-Bagaric/Dialog-Center-Text",
	`progress2`:     "Southclaws/progress2",
	`zcmd`:          "Southclaws/zcmd",
	`formatex`:      "Southclaws/formatex",
	`modio`:         "Southclaws/modio",
	`ini`:           "Southclaws/ini",
	`a_mysql`:       "pBlueG/SA-MP-MySQL",
	`sscanf2`:       "maddinat0r/sscanf",
	`ctime`:         "Southclaws/ctime",
	`redis`:         "Southclaws/samp-redis",
	`sqlitei`:       "oscar-broman/sqlitei",
	`strlib`:        "oscar-broman/strlib",
	`weapon-config`: "oscar-broman/samp-weapon-config",
	`md-sort`:       "oscar-broman/md-sort",
	`logger`:        "Southclaws/samp-logger",
	`crashdetect`:   "AmyrAhmady/samp-plugin-crashdetect",
}

// FindIncludes checks a list of files and scans the contents searching for includes with known dependency strings
func FindIncludes(files []string) (includes []versioning.DependencyString) {
	mux := &sync.Mutex{}
	seen := map[versioning.DependencyString]struct{}{}
	wg := sync.WaitGroup{}
	for _, file := range files {
		wg.Add(1)
		go func(innerFile string) {
			content, err := ioutil.ReadFile(innerFile)
			if err != nil {
				print.Erro(err)
				return
			}

			for expr := range IncludesToDependencies {
				if len(regexp.MustCompile(fmt.Sprintf(`#include\s\<%s\>.*`, expr)).FindAllString(string(content), -1)) > 0 {
					if _, ok := seen[IncludesToDependencies[expr]]; !ok {
						mux.Lock()
						includes = append(includes, IncludesToDependencies[expr])
						mux.Unlock()
						print.Info("Discovered include matching:", expr, " resolved to:", IncludesToDependencies[expr])
					}
				}
			}
			wg.Done()
		}(file)
	}
	wg.Wait()
	sort.Slice(includes, func(i, j int) bool { return includes[i] < includes[j] })
	return
}
