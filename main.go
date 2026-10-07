package main

import (
	"fmt"
	"os"
	"regexp"
)

var decoLine = regexp.MustCompile(`(?m)^(\s*)@(\w+.*)$`)

func main() {
	path := "main.gox"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	goSrc := decoLine.ReplaceAll(src, []byte("$1//goxnest:$2"))

	fmt.Print(string(goSrc))
}
