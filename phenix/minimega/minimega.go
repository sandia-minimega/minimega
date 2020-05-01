package minimega

import "flag"

var f_minimegaBase string

func init() {
	flag.StringVar(&f_minimegaBase, "minimega-base", "/tmp/minimega", "base path for minimega")
}
