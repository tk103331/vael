// vael-core runs without a graphical environment, including on an SSH host.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"vael/internal/core"
)

func main() {
	stdio := flag.Bool("stdio", false, "serve the editor core over stdin/stdout")
	version := flag.Bool("version", false, "say which version this core is")
	flag.Parse()
	if *version {
		fmt.Printf("vael-core %s\n", core.Version)
		return
	}
	if !*stdio {
		flag.Usage()
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	go func() { <-ctx.Done(); _ = os.Stdin.Close() }()
	if err := core.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
