package main

import (
	"context"
	"os"

	"github.com/shaharia-lab/goai/mcp"
)

func main() {
	server := mcp.NewStdIOServer(os.Stdin, os.Stdout)
	ctx := context.Background()
	if err := server.Run(ctx); err != nil {
		panic(err)
	}
}
