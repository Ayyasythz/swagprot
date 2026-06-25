// Command embed-fiber shows swagprot mounted under "/swagger" inside a Fiber
// (fasthttp) application — the same shape as a service like nocturna-ta/ums,
// which serves a Fiber REST API and a gRPC server.
//
//	go run ./examples/greeter        # gRPC server on :50051 (stands in for ums :35000)
//	go run .                         # this Fiber app on :8900
//	open http://localhost:8900/swagger/
//
// Note on streaming: Fiber is built on fasthttp, whose net/http adaptor cannot
// upgrade WebSocket connections. swagprot detects this and the UI automatically
// falls back to a buffered HTTP endpoint, so streaming "try it out" still works
// (responses arrive together at the end rather than live). For live streaming,
// run swagprot as a sidecar or on its own net/http listener (see examples/embed-http).
package main

import (
	"context"
	"log"
	"time"

	"github.com/bbyasyi/swagprot"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Point at your gRPC server. In ums this would be localhost:35000.
	swag, err := swagprot.NewServer(ctx, swagprot.Config{
		Dial:     swagprot.DialOptions{Address: "localhost:50051", Plaintext: true},
		BasePath: "/swagger",
	})
	if err != nil {
		log.Fatalf("swagprot: %v", err)
	}
	defer swag.Close()

	app := fiber.New()
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })

	// Fiber passes the full request path (including "/swagger") to the wrapped
	// net/http handler; swagprot strips it via BasePath. Use is terminal here
	// because the adaptor does not call c.Next().
	app.Use("/swagger", adaptor.HTTPHandler(swag))

	log.Println("listening on http://localhost:8900  (UI at /swagger/)")
	log.Fatal(app.Listen(":8900"))
}
