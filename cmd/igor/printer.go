// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type rowPrinter struct {
	w      io.Writer
	filter func(*Reservation) bool

	nameFmt string
	timeFmt string

	showColors bool
}

type tablePrinter struct {
	filter func(*Reservation) bool
	alive  map[int]bool

	showColors bool
}

func (p rowPrinter) printHeader() {
	name := fmt.Sprintf(p.nameFmt, "NAME")
	if p.showColors {
		name = BgBlack + FgWhite + name + Reset
	}

	fmt.Fprintln(p.w,
		name, "\t",
		"OWNER", "\t",
		"START", "\t",
		"END", "\t",
		"FLAGS", "\t",
		"SIZE", "\t",
		"NODES")
}

func (p rowPrinter) printSpacer() {
	name := strings.Replace(fmt.Sprintf(p.nameFmt, ""), " ", "-", -1)
	if p.showColors {
		name = BgBlack + FgWhite + name + Reset
	}

	fmt.Fprintln(p.w,
		name, "\t",
		"-------", "\t",
		"------------", "\t",
		"------------", "\t",
		"------", "\t",
		"-----", "\t",
		"------------")
}

func (p rowPrinter) printHosts(name, color string, hosts []string) {
	name = fmt.Sprintf(p.nameFmt, name)
	if p.showColors {
		name = color + name + Reset
	}

	fmt.Fprintln(p.w,
		name, "\t",
		"N/A", "\t",
		"N/A", "\t",
		"N/A", "\t",
		"N/A", "\t",
		len(hosts), "\t",
		igor.unsplitRange(hosts))
}

func (p rowPrinter) printReservations(rs []*Reservation) {
	for i, r := range rs {
		if p.filter(r) {
			continue
		}

		name := fmt.Sprintf(p.nameFmt, r.Name)
		if p.showColors {
			name = colorize(name, i)
		}

		fmt.Fprintln(p.w,
			name, "\t",
			r.Owner, "\t",
			r.Start.Format(p.timeFmt), "\t",
			r.End.Format(p.timeFmt), "\t",
			r.Flags(igor.Now), "\t",
			len(r.Hosts), "\t",
			igor.unsplitRange(r.Hosts))
	}
}

func (p tablePrinter) printTable(resarray []*Reservation) {
	// figure out how many digits we need per node displayed
	nodewidth := len(strconv.Itoa(igor.End))
	nodefmt := "%" + strconv.Itoa(nodewidth) + "v"

	// how many nodes per rack?
	perrack := igor.Rackwidth * igor.Rackheight

	// How wide is the full rack display?
	// width of nodes * number of nodes across a rack, plus the number of | characters we need
	totalwidth := nodewidth*igor.Rackwidth + igor.Rackwidth + 1

	// figure out all the node -> reservations ahead of time
	n2r := map[int]int{}
	for i, r := range resarray {
		if r.Start.Before(igor.Now) {
			for _, name := range r.Hosts {
				name := strings.TrimPrefix(name, igor.Prefix)
				v, err := strconv.Atoi(name)
				if err == nil {
					n2r[v] = i
				}
			}
		}
	}

	var buf bytes.Buffer

	write := func(color, s string) {
		if p.showColors {
			buf.WriteString(color)
		}

		buf.WriteString(s)

		if p.showColors {
			buf.WriteString(Reset)
		}
	}

	for i := igor.Start; i <= igor.End; i += perrack {
		// print top bar
		for j := 0; j < totalwidth; j++ {
			write(Reverse, "-")
		}
		buf.WriteString("\n")

		for j := i; j < i+perrack; j++ {
			// starting a new row
			if (j-1)%igor.Rackwidth == 0 {
				write(Reverse, "|")
			}

			if j <= igor.End {
				name := fmt.Sprintf(nodefmt, j)
				if index, ok := n2r[j]; ok {
					filtered := p.filter(resarray[index])

					if p.showColors {
						if !p.alive[j] {
							write(BgRed, name)
						} else if filtered {
							write(Dim, name)
						} else {
							buf.WriteString(colorize(name, index))
						}
					} else {
						buf.WriteString(name)
					}
				} else {
					if p.alive[j] {
						buf.WriteString(name)
					} else {
						write(BgRed, name)
					}
				}
			} else {
				fmt.Fprintf(&buf, nodefmt, " ")
			}
			write(Reverse, "|")

			// end of row
			if (j-1)%igor.Rackwidth == igor.Rackwidth-1 {
				buf.WriteString("\n")
			}
		}

		// print bottom bar
		for j := 0; j < totalwidth; j++ {
			write(Reverse, "-")
		}
		buf.WriteString("\n\n")
	}

	fmt.Print(buf.String())
}
