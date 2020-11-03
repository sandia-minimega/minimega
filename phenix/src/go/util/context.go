package util

import "context"

type warningsKey struct{}

func AddWarnings(ctx context.Context, warns ...error) context.Context {
	warnings, _ := ctx.Value(warningsKey{}).([]error)
	warnings = append(warnings, warns...)
	return context.WithValue(ctx, warningsKey{}, warnings)
}

func Warnings(ctx context.Context) []error {
	warns, _ := ctx.Value(warningsKey{}).([]error)
	return warns
}
