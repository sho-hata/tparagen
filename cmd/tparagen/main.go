package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sho-hata/tparagen"
)

func main() {
	now := time.Now()
	if err := tparagen.Run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	if time.Since(now).Seconds() < 0.01 {
		fmt.Printf("Done in %vms\n", time.Since(now).Milliseconds())
	} else {
		fmt.Printf("Done in %vs\n", time.Since(now).Seconds())
	}
}
