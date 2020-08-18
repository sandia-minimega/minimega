package cluster

import (
	"fmt"
	"os"
	"strings"

	"phenix/internal/mm/mmcli"
)

var hostnameSuffixes string

func isHeadnode(node string) bool {
	headnode, _ := os.Hostname()

	// Trim host name suffixes (like -minimega, or -gophenix) potentially added to
	// Docker containers by Docker Compose config.
	for _, s := range strings.Split(hostnameSuffixes, ",") {
		headnode = strings.TrimSuffix(headnode, s)
	}

	// Trim node name suffixes (like -minimega, or -gophenix) potentially added to
	// Docker containers by Docker Compose config.
	for _, s := range strings.Split(hostnameSuffixes, ",") {
		node = strings.TrimSuffix(node, s)
	}

	return node == headnode
}

func getVMHost(exp, vm string) (string, error) {
	cmd := mmcli.NewNamespacedCommand(exp)
	cmd.Command = "vm info"
	cmd.Columns = []string{"host"}
	cmd.Filters = []string{"name=" + fmt.Sprintf("%s_%s", exp, vm)}

	status := mmcli.RunTabular(cmd)

	if len(status) == 0 {
		return "", fmt.Errorf("VM %s not found", vm)
	}

	return status[0]["host"], nil
}
