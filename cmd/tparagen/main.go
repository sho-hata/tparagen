package main

import (
	"fmt"
	"os"

	"github.com/sho-hata/tparagen"
)

func main() {
	if err := tparagen.Run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}

	fmt.Println("done!")
}
