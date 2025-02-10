package main

import (
	"context"
	"flag"
	"github.com/shaharia-lab/goai/mcp"
	"log"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	flag.Parse()

	logger := log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)

	server := mcp.NewSSEServer(mcp.NewCommonServer(
		mcp.UseLogger(log.New(os.Stderr, "[MCP SSEServer] ", log.LstdFlags|log.Lmsgprefix)),
	))
	server.SetAddress(*addr)

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

/*func main() {
	server := mcp.NewStdIOServer(os.Stdin, os.Stdout)
	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}*/
