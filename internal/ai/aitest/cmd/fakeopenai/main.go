// fakeopenai is an OpenAI-compatible server without a model. It streams a
// canned Markdown answer that echoes the last user message, which is enough
// to try the kubedojo assistant end to end:
//
//	go run ./internal/ai/aitest/cmd/fakeopenai -addr 127.0.0.1:4497
//	kubedojo up --ai --ai-provider openai --ai-base-url http://127.0.0.1:4497/v1 --ai-model fake
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/Mistral-valaise/kubedojo/internal/ai/aitest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:4497", "listen address")
	delay := flag.Duration("delay", 30*time.Millisecond, "pause between two pieces of the answer")
	flag.Parse()
	srv := &http.Server{Addr: *addr, Handler: &aitest.Fake{ChunkDelay: *delay}, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("fake OpenAI-compatible server on http://%s/v1 (POST /v1/chat/completions)", *addr)
	log.Fatal(srv.ListenAndServe())
}
