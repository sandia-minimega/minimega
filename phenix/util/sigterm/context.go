package sigterm

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func CancelContext(ctx context.Context) context.Context {
	ctxWithCancel, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()

		term := make(chan os.Signal, 1)
		signal.Notify(term, syscall.SIGTERM, syscall.SIGINT)

		select {
		case <-term:
		case <-ctx.Done():
		}
	}()

	return ctxWithCancel
}
