package util

import "context"

type warningsKey struct{}

func AddWarnings(ctx context.Context, warns []error) context.Context {
	return context.WithValue(ctx, warningsKey{}, warns)
}

func Warnings(ctx context.Context) []error {
	warns, _ := ctx.Value(warningsKey{}).([]error)
	return warns
}
