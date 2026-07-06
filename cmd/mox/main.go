package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/bthall/mox/internal/cli"
)

func main() {
	os.Exit(run())
}

// run exists so that deferred cleanup executes before os.Exit, which would
// otherwise skip it.
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return cli.Execute(ctx)
}
