// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

// Some color constants for output
const (
	Reset      = "\x1b[0000m"
	Bright     = "\x1b[0001m"
	Dim        = "\x1b[0002m"
	Underscore = "\x1b[0004m"
	Blink      = "\x1b[0005m"
	Reverse    = "\x1b[0007m"
	Hidden     = "\x1b[0008m"

	FgBlack   = "\x1b[0030m"
	FgRed     = "\x1b[0031m"
	FgGreen   = "\x1b[0032m"
	FgYellow  = "\x1b[0033m"
	FgBlue    = "\x1b[0034m"
	FgMagenta = "\x1b[0035m"
	FgCyan    = "\x1b[0036m"
	FgWhite   = "\x1b[0037m"

	FgLightWhite = "\x1b[0097m"

	BgBlack         = "\x1b[0040m"
	BgRed           = "\x1b[0041m"
	BgGreen         = "\x1b[0042m"
	BgYellow        = "\x1b[0043m"
	BgBlue          = "\x1b[0044m"
	BgMagenta       = "\x1b[0045m"
	BgCyan          = "\x1b[0046m"
	BgWhite         = "\x1b[0047m"
	BgBrightBlack   = "\x1b[0100m"
	BgBrightRed     = "\x1b[0101m"
	BgBrightGreen   = "\x1b[0102m"
	BgBrightYellow  = "\x1b[0103m"
	BgBrightBlue    = "\x1b[0104m"
	BgBrightMagenta = "\x1b[0105m"
	BgBrightCyan    = "\x1b[0106m"
	BgBrightWhite   = "\x1b[0107m"
)

var fgColors = []string{
	FgLightWhite,
	FgLightWhite,
	FgLightWhite,
	FgLightWhite,
	FgBlack,
	FgBlack,
	FgBlack,
	FgBlack,
	FgBlack,
	FgBlack,
}

var bgColors = []string{
	BgGreen,
	BgBlue,
	BgMagenta,
	BgCyan,
	BgYellow,
	BgBrightGreen,
	BgBrightBlue,
	BgBrightMagenta,
	BgBrightCyan,
	BgBrightYellow,
}

func colorize(str string, index int) string {
	return fgColors[index%len(fgColors)] + bgColors[index%len(bgColors)] + str + Reset
}
