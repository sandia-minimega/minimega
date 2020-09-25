package cmd

import (
	"fmt"

	"github.com/spf13/pflag"
)

func MustGetString(flags *pflag.FlagSet, name string) string {
	val, err := flags.GetString(name)
	if err != nil {
		panic(fmt.Sprintf("Getting value for %s: %v", name, err))
	}

	return val
}

func MustGetBool(flags *pflag.FlagSet, name string) bool {
	val, err := flags.GetBool(name)
	if err != nil {
		panic(fmt.Sprintf("Getting value for %s: %v", name, err))
	}

	return val
}

func MustGetInt(flags *pflag.FlagSet, name string) int {
	val, err := flags.GetInt(name)
	if err != nil {
		panic(fmt.Sprintf("Getting value for %s: %v", name, err))
	}

	return val
}
