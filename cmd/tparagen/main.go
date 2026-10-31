package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/sho-hata/tparagen"
)

var (
	ignoreDirectories = kingpin.Flag("ignore", "ignore directory names. ex: foo,bar,baz\n(testdata directory is always ignored.)").String()
	verbose           = kingpin.Flag("verbose", "print a line for every file that was changed or has tests left sequential.").Short('v').Bool()
	minGoVersion      = kingpin.Flag("min-go-version", "minimum go version of the target code. ex: 1.21\n(default: the go directive of the nearest go.mod. Loop variables are copied before parallel subtests when it is older than 1.22.)").String()
)

func main() {
	now := time.Now()

	// Must be set before Parse(), or -h is rejected as an unknown flag.
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	// Cancel the run gracefully on interruption (Ctrl+C) or termination (kill),
	// so temporary files are cleaned up instead of being left behind.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := tparagen.Run(ctx, os.Stdout, os.Stderr, strings.Split(*ignoreDirectories, ","), *minGoVersion, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if time.Since(now).Seconds() < 0.01 {
		fmt.Printf("✨ Done in %dms\n", time.Since(now).Milliseconds())
	} else {
		fmt.Printf("✨ Done in %ss\n", fmt.Sprintf("%.2f", time.Since(now).Seconds()))
	}
}
