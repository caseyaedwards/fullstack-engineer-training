// Command hello prints one line and exits. It is the program that the
// Containers track Dockerfile builds and runs.
package main

import "fmt"

// region: main
func main() {
	fmt.Println("hello from a distroless container")
}

// endregion: main
