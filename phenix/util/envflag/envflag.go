package envflag

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func Parse(p string) {
	Update(p, flag.CommandLine)
}

func Update(p string, fs *flag.FlagSet) {
	set := make(map[string]interface{})

	// build map of flags set explicitly
	fs.Visit(func(f *flag.Flag) {
		set[f.Name] = nil
	})

	fs.VisitAll(func(f *flag.Flag) {
		if f.Name == "help" {
			return
		}

		// create env var name based on supplied prefix
		envVar := fmt.Sprintf("%s_%s", p, strings.ToUpper(f.Name))
		envVar = strings.ReplaceAll(envVar, "-", "_")
		envVar = strings.ReplaceAll(envVar, ".", "_")

		// update Flag.Value if env var is set
		if val := os.Getenv(envVar); val != "" {
			if _, ok := set[f.Name]; !ok {
				fs.Set(f.Name, val)
			}
		}

		// append env var to Flag.Usage field
		f.Usage = fmt.Sprintf("%s [%s]", f.Usage, envVar)
	})
}
