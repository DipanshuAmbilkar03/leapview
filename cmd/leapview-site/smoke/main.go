// Command smoke verifies that a running LeapView public site serves its
// complete map-asset package. It is the post-deploy counterpart of the
// in-process delivery tests.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/flidai/leapview/internal/app/site/mapassets"
)

func main() {
	baseURL := flag.String("base-url", "https://leapview.dev", "site origin to verify")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall verification timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}
	if err := mapassets.Verify(ctx, client, *baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "map assets are not served by %s:\n%v\n", *baseURL, err)
		os.Exit(1)
	}
	fmt.Printf("map assets served by %s\n", *baseURL)
}
