// Command mockserver runs the BSAL provider fixture server standalone, bound
// to a real port, for manual testing or a CI job that wants a sidecar rather
// than an in-process httptest.Server. `go test` itself doesn't use this —
// provider tests call mockserver.NewServer directly.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/Business-Aware-Control-Plane/business-signal-engine/tools/mockserver"
)

func main() {
	addr := flag.String("addr", ":8089", "address to bind the fixture server on")
	scenario := flag.String("scenario", string(mockserver.ScenarioSuccess), "success | empty | malformed | provider_error | no_content")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "[mockserver] listening on %s (scenario=%s)\n", *addr, *scenario)
	log.Fatal(http.ListenAndServe(*addr, mockserver.NewMux(mockserver.Scenario(*scenario))))
}
