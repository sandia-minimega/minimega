package ron

import (
	"regexp"
	"strconv"
	"strings"
)

var regex = regexp.MustCompile(`^(v|V)`)

func majorVersion(v string) int {
	parts := versionParts(v)

	if len(parts) < 1 {
		return 0
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}

	return major
}

func minorVersion(v string) int {
	parts := versionParts(v)

	if len(parts) < 2 {
		return 0
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}

	return minor
}

func patchVersion(v string) int {
	parts := versionParts(v)

	if len(parts) < 3 {
		return 0
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0
	}

	return patch
}

func versionParts(v string) []string {
	v = regex.ReplaceAllString(v, "")
	return strings.Split(v, ".")
}
