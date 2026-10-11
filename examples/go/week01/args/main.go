// The first cut of the Week 1 monitor: it reads the URL from os.Args and
// prints every argument it received. Run it from the examples/go directory:
//
//	go run ./week01/args https://example.com

// region: main
package main

import (
	"fmt"
	"os"
)

func main() {
	// os.Args[0] is the program itself; the URL is the first real argument.
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: your-monitor URL")
		os.Exit(2)
	}
	for i, arg := range os.Args {
		fmt.Printf("os.Args[%d] = %s\n", i, arg)
	}
	url := os.Args[1]
	fmt.Println("checking", url)
}

// endregion: main
