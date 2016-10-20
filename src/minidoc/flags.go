package main

import (
	"flag"
)

var (
	f_server   = flag.String("server", ":9003", "HTTP server \"host:port\"")
	f_root     = flag.String("root", "doc/content/", "HTTP root directory")
	f_base     = flag.String("base", "doc/template/", "base path for static content and templates")
	f_exec     = flag.Bool("exec", false, "allow minimega commands")
	f_loglevel = flag.String("level", "debug", "log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "log to file")
	f_minimega = flag.String("minimega", "/tmp/minimega", "path to minimega base directory")
)
