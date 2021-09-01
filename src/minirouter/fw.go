package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"minicli"
	log "minilog"
)

func init() {
	minicli.Register(&minicli.Handler{
		Patterns: []string{
			"fw <default,> <accept,drop,reject>",
			"fw <accept,drop,reject> <in,out> <index> <dst> <proto>",
			"fw <accept,drop,reject> <in,out> <index> <src> <dst> <proto>",
			"fw chain <chain> <default,> action <accept,drop,reject>",
			"fw chain <chain> action <accept,drop,reject> <dst> <proto>",
			"fw chain <chain> action <accept,drop,reject> <src> <dst> <proto>",
			"fw chain <chain> apply <in,out> <index>",
			"fw <flush,>",
		},
		Call: handleFW,
	})
}

func handleChain(c *minicli.Command, r chan<- minicli.Responses) error {
	name := c.StringArgs["chain"]

	if out, err := exec.Command("iptables", "-N", name).CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "exists") { // error isn't due to existing chain
			return fmt.Errorf("creating %s chain %v: %v", name, err, string(out))
		}
	}

	if c.BoolArgs["default"] {
		if c.BoolArgs["accept"] {
			if out, err := exec.Command("iptables", "-A", name, "-j", "ACCEPT").CombinedOutput(); err != nil {
				return fmt.Errorf("defaulting %s chain to ACCEPT %v: %v", name, err, string(out))
			}
		} else if c.BoolArgs["drop"] {
			if out, err := exec.Command("iptables", "-A", name, "-j", "DROP").CombinedOutput(); err != nil {
				return fmt.Errorf("defaulting %s chain to DROP %v: %v", name, err, string(out))
			}
		} else if c.BoolArgs["reject"] {
			if out, err := exec.Command("iptables", "-A", name, "-j", "REJECT").CombinedOutput(); err != nil {
				return fmt.Errorf("defaulting %s chain to REJECT %v: %v", name, err, string(out))
			}
		}

		return nil
	}

	if c.BoolArgs["accept"] || c.BoolArgs["drop"] || c.BoolArgs["reject"] {
		rule := []string{name}

		if proto := c.StringArgs["proto"]; proto != "" {
			rule = append(rule, "-p", proto)
		}

		if src := c.StringArgs["src"]; src != "" {
			fields := strings.Split(src, ":")

			switch len(fields) {
			case 1:
				rule = append(rule, "-s", src)
			case 2:
				if _, err := strconv.Atoi(fields[1]); err != nil {
					return fmt.Errorf("converting source port: %v", err)
				}

				if proto := c.StringArgs["proto"]; proto == "" {
					return fmt.Errorf("must specify proto when specifying ports")
				}

				rule = append(rule, "-s", fields[0], "--sport", fields[1])
			default:
				return fmt.Errorf("malformed source")
			}
		}

		if dst := c.StringArgs["dst"]; dst != "" {
			fields := strings.Split(dst, ":")

			switch len(fields) {
			case 1:
				rule = append(rule, "-d", dst)
			case 2:
				if _, err := strconv.Atoi(fields[1]); err != nil {
					return fmt.Errorf("converting destination port: %v", err)
				}

				if proto := c.StringArgs["proto"]; proto == "" {
					return fmt.Errorf("must specify proto when specifying ports")
				}

				rule = append(rule, "-d", fields[0], "--dport", fields[1])
			default:
				return fmt.Errorf("malformed destination")
			}
		}

		if c.BoolArgs["accept"] {
			rule = append(rule, "-j", "ACCEPT")
		} else if c.BoolArgs["drop"] {
			rule = append(rule, "-j", "DROP")
		} else if c.BoolArgs["reject"] {
			rule = append(rule, "-j", "REJECT")
		}

		return addRule(rule)
	}

	if c.BoolArgs["in"] || c.BoolArgs["out"] {
		var (
			idx   = -1
			iface = "lo"
			err   error
		)

		if c.StringArgs["index"] != "lo" {
			idx, err = strconv.Atoi(c.StringArgs["index"])
			if err != nil {
				return fmt.Errorf("converting interface index: %v", err)
			}
		}

		if idx != -1 {
			// get interface name using the index
			if iface, err = findEth(idx); err != nil {
				return fmt.Errorf("getting interface name for index: %v", err)
			}
		}

		var rule []string

		if c.BoolArgs["in"] {
			rule = []string{"FORWARD", "-i", iface, "-j", name}
		} else {
			rule = []string{"FORWARD", "-o", iface, "-j", name}
		}

		return addRule(rule)
	}

	return nil
}

func handleFW(c *minicli.Command, r chan<- minicli.Responses) {
	defer func() {
		r <- nil
	}()

	if c.BoolArgs["flush"] {
		log.Debug("flushing firwall")

		if err := flushFW(); err != nil {
			log.Errorln(err)
		}

		return
	}

	if c.StringArgs["chain"] != "" {
		if err := handleChain(c, r); err != nil {
			log.Errorln(err)
		}

		return
	}

	if c.BoolArgs["default"] {
		if c.BoolArgs["accept"] {
			if out, err := exec.Command("iptables", "-P", "FORWARD", "ACCEPT").CombinedOutput(); err != nil {
				log.Error("defaulting FORWARD to ACCEPT %v: %v", err, string(out))
				return
			}
		} else if c.BoolArgs["drop"] {
			if out, err := exec.Command("iptables", "-P", "FORWARD", "DROP").CombinedOutput(); err != nil {
				log.Error("defaulting FORWARD to DROP %v: %v", err, string(out))
				return
			}
		} else if c.BoolArgs["reject"] {
			if out, err := exec.Command("iptables", "-P", "FORWARD", "REJECT").CombinedOutput(); err != nil {
				log.Error("defaulting FORWARD to REJECT %v: %v", err, string(out))
				return
			}
		}

		return
	}

	if c.BoolArgs["accept"] || c.BoolArgs["drop"] || c.BoolArgs["reject"] {
		var (
			idx   = -1
			iface = "lo"
			err   error
		)

		if c.StringArgs["index"] != "lo" {
			idx, err = strconv.Atoi(c.StringArgs["index"])
			if err != nil {
				log.Error("converting interface index: %v", err)
				return
			}
		}

		if idx != -1 {
			// get interface name using the index
			if iface, err = findEth(idx); err != nil {
				log.Error("getting interface name for index: %v", err)
				return
			}
		}

		var rule []string

		if c.BoolArgs["in"] {
			rule = []string{"FORWARD", "-i", iface}
		} else {
			rule = []string{"FORWARD", "-o", iface}
		}

		if proto := c.StringArgs["proto"]; proto != "" {
			rule = append(rule, "-p", proto)
		}

		if src := c.StringArgs["src"]; src != "" {
			fields := strings.Split(src, ":")

			switch len(fields) {
			case 1:
				rule = append(rule, "-s", src)
			case 2:
				if _, err = strconv.Atoi(fields[1]); err != nil {
					log.Error("converting source port: %v", err)
					return
				}

				if proto := c.StringArgs["proto"]; proto == "" {
					log.Errorln("must specify proto when specifying ports")
					return
				}

				rule = append(rule, "-s", fields[0], "--sport", fields[1])
			default:
				log.Errorln("malformed source")
				return
			}
		}

		if dst := c.StringArgs["dst"]; dst != "" {
			fields := strings.Split(dst, ":")

			switch len(fields) {
			case 1:
				rule = append(rule, "-d", dst)
			case 2:
				if _, err = strconv.Atoi(fields[1]); err != nil {
					log.Error("converting destination port: %v", err)
					return
				}

				if proto := c.StringArgs["proto"]; proto == "" {
					log.Errorln("must specify proto when specifying ports")
					return
				}

				rule = append(rule, "-d", fields[0], "--dport", fields[1])
			default:
				log.Errorln("malformed destination")
				return
			}
		}

		if c.BoolArgs["accept"] {
			rule = append(rule, "-j", "ACCEPT")
		} else if c.BoolArgs["drop"] {
			rule = append(rule, "-j", "DROP")
		} else if c.BoolArgs["reject"] {
			rule = append(rule, "-j", "REJECT")
		}

		if err := addRule(rule); err != nil {
			log.Errorln(err)
			return
		}
	}
}

func addRule(rule []string) error {
	args := append([]string{"-A"}, rule...)

	out, err := exec.Command("iptables", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("adding iptables rule (%s) %v: %v", strings.Join(rule, " "), err, string(out))
	}

	return nil
}

func flushFW() error {
	if out, err := exec.Command("iptables", "-F").CombinedOutput(); err != nil {
		return fmt.Errorf("flushing rules %v: %v", err, string(out))
	}

	if out, err := exec.Command("iptables", "-X").CombinedOutput(); err != nil {
		return fmt.Errorf("deleting custom chains %v: %v", err, string(out))
	}

	if err := defaultAllAccept(); err != nil {
		return err
	}

	if err := createEstablishedChain(); err != nil {
		return err
	}

	return nil
}

func defaultAllAccept() error {
	if out, err := exec.Command("iptables", "-P", "INPUT", "ACCEPT").CombinedOutput(); err != nil {
		return fmt.Errorf("defaulting INPUT to ACCEPT %v: %v", err, string(out))
	}

	if out, err := exec.Command("iptables", "-P", "FORWARD", "ACCEPT").CombinedOutput(); err != nil {
		return fmt.Errorf("defaulting FORWARD to ACCEPT %v: %v", err, string(out))
	}

	if out, err := exec.Command("iptables", "-P", "OUTPUT", "ACCEPT").CombinedOutput(); err != nil {
		return fmt.Errorf("defaulting OUTPUT to ACCEPT %v: %v", err, string(out))
	}

	return nil
}

func createEstablishedChain() error {
	if out, err := exec.Command("iptables", "-N", "ESTABLISHED").CombinedOutput(); err != nil {
		return fmt.Errorf("creating custom ESTABLISHED chain %v: %v", err, string(out))
	}

	rule := "-A ESTABLISHED -p %s -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT"

	for _, proto := range []string{"tcp", "udp", "icmp"} {
		args := strings.Fields(fmt.Sprintf(rule, proto))

		if out, err := exec.Command("iptables", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("creating custom ESTABLISHED chain %v: %v", err, string(out))
		}
	}

	if out, err := exec.Command("iptables", "-A", "ESTABLISHED", "-j", "RETURN").CombinedOutput(); err != nil {
		return fmt.Errorf("creating custom ESTABLISHED chain %v: %v", err, string(out))
	}

	if out, err := exec.Command("iptables", "-A", "FORWARD", "-j", "ESTABLISHED").CombinedOutput(); err != nil {
		return fmt.Errorf("creating custom ESTABLISHED chain %v: %v", err, string(out))
	}

	return nil
}
