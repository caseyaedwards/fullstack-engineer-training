// Command outside sits outside your-monitor and tries to import one of its
// internal packages. The go command refuses to build it: layout_test.go
// checks the error. The go command skips testdata folders in ./...
// patterns, which keeps this failing build out of go vet ./... and
// go test ./....
package main

import (
	"fmt"

	"fsp/examples/week06/your-monitor/internal/check"
)

func main() {
	fmt.Println(check.Result{URL: "https://example.com"})
}
