package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/ndeloof/go-garmin/pkg/garmin"
	"github.com/ndeloof/go-garmin/pkg/mcp"
)

// cmdMCP runs the MCP server over stdio. Protocol traffic uses stdin/stdout;
// diagnostics go to stderr so they never corrupt the JSON-RPC stream.
func cmdMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	tokens := tokensFlag(fs)
	_ = fs.Parse(args)

	store := garmin.NewFileTokenStore(*tokens)
	client, err := garmin.NewClientFromStore(ctx, store)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "go-garmin MCP server: tokens %s\n", store.Path())
	return mcp.NewServer(client).ServeStdio(ctx, os.Stdin, os.Stdout)
}
