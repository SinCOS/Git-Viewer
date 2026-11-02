// Command gitviewer-helper is the small resident Git executor uploaded to an
// SSH host by Git Viewer. It only serves the framed fixed-argv protocol; it
// never accepts a shell command.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"gitviewer/internal/remotehelper"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout))
}

func run(args []string, in io.Reader, out io.Writer) int {
	flags := flag.NewFlagSet("gitviewer-helper", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	serve := flags.Bool("serve", false, "serve the framed protocol on stdin/stdout")
	version := flags.Bool("version", false, "print the helper version and platform")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*serve && *version) {
		fmt.Fprintln(os.Stderr, "gitviewer-helper: invalid arguments")
		return 2
	}
	if *version {
		fmt.Fprintf(out, "gitviewer-helper %d %s/%s\n", remotehelper.Proto, runtime.GOOS, runtime.GOARCH)
		return 0
	}
	if !*serve {
		fmt.Fprintln(os.Stderr, "gitviewer-helper: use --serve")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := remotehelper.Serve(ctx, in, out, remotehelper.NewEngine("")); err != nil {
		fmt.Fprintln(os.Stderr, "gitviewer-helper: protocol error")
		return 1
	}
	return 0
}
