// Command embed-http shows swagprot mounted under a sub-path ("/swagger") in a
// plain net/http server alongside your own routes. With net/http the streaming
// "try it out" works over WebSockets with full fidelity.
//
//	go run ./examples/greeter                       # a gRPC server on :50051
//	go run ./examples/embed-http                     # this app on :8900
//	open http://localhost:8900/swagger/
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bbyasyi/swagprot"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Build the swagprot handler, told it lives under "/swagger".
	swag, err := swagprot.NewServer(ctx, swagprot.Config{
		Dial:     swagprot.DialOptions{Address: "localhost:50051", Plaintext: true},
		BasePath: "/swagger",
	})
	if err != nil {
		log.Fatalf("swagprot: %v", err)
	}
	defer swag.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	// Mount the explorer. The "/swagger/" subtree (and a redirect from
	// "/swagger") is handled by swagprot itself.
	mux.Handle("/swagger/", swag)
	mux.Handle("/swagger", swag)

	log.Println("listening on http://localhost:8900  (UI at /swagger/)")
	log.Fatal(http.ListenAndServe(":8900", mux))
}
