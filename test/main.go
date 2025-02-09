package main

import (
	"context"
	"github.com/shaharia-lab/goai/mcp"
	"os"
)

/*func main() {
	addr := flag.String("addr", ":8080", "HTTP address to listen on")
	flag.Parse()

	server := mcp.NewSSEServer()
	server.SetAddress(*addr)

	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}*/

func main() {
	server := mcp.NewStdIOServer(os.Stdin, os.Stdout)
	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
