package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/sho-hata/tparagen"
)

var (
	ignoreDirectories = kingpin.Flag("ignore", "ignore directory names. ex: foo,bar,baz\n(testdata directory is always ignored.)").String()
)

func main() {
	now := time.Now()

	kingpin.Parse()
	kingpin.HelpFlag.Short('h')

	if err := tparagen.Run(os.Stdout, os.Stderr, strings.Split(*ignoreDirectories, ",")); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if time.Since(now).Seconds() < 0.01 {
		fmt.Printf("✨ Done in %dms\n", time.Since(now).Milliseconds())
	} else {
		fmt.Printf("✨ Done in %ss\n", fmt.Sprintf("%.2f", time.Since(now).Seconds()))
	}
}
