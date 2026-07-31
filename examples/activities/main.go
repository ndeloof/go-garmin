// Example: list the 10 most recent activities using tokens obtained with the
// garmin CLI (cmd/garmin login).
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ndeloof/go-garmin/pkg/garmin"
)

func main() {
	ctx := context.Background()

	// $GARMINTOKENS or ~/.garminconnect/garmin_tokens.json; rotated tokens
	// are persisted back to the file.
	store := garmin.NewFileTokenStore("")
	client, err := garmin.NewClientFromStore(ctx, store)
	if err != nil {
		log.Fatal(err)
	}

	count := 0
	for act, err := range client.Activities.All(ctx, nil) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%-12d %-19s %-16s %6.1f km  %s\n",
			act.ActivityID, act.StartTimeLocal, act.ActivityType.TypeKey,
			act.Distance/1000, act.ActivityName)
		if count++; count == 10 {
			break
		}
	}
}
