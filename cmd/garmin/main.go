// Command garmin obtains and manages the Garmin Connect tokens used by the
// go-garmin library (and by python-garminconnect — the token file format is
// shared).
//
//	garmin login   [--tokens <path>]   interactive login (email, password, 2FA)
//	garmin whoami  [--tokens <path>]   verify tokens by fetching the profile
//	garmin refresh [--tokens <path>]   force a token refresh (persists rotation)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

func main() {
	flag.Usage = usage
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var err error
	switch flag.Arg(0) {
	case "login":
		err = cmdLogin(ctx, flag.Args()[1:])
	case "whoami":
		err = cmdWhoami(ctx, flag.Args()[1:])
	case "refresh":
		err = cmdRefresh(ctx, flag.Args()[1:])
	case "", "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "garmin: unknown command %q\n\n", flag.Arg(0))
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "garmin:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: garmin <command> [flags]

Commands:
  login     log in to Garmin Connect (email, password, 2FA) and write the token file
  whoami    verify the stored tokens by fetching the Garmin profile
  refresh   force an access-token refresh and persist the rotated refresh token

Flags (all commands):
  --tokens <path>   token file (default: $GARMINTOKENS or ~/.garminconnect/garmin_tokens.json)
`)
}

func tokensFlag(fs *flag.FlagSet) *string {
	return fs.String("tokens", "", "token file path (default: $GARMINTOKENS or ~/.garminconnect/garmin_tokens.json)")
}
