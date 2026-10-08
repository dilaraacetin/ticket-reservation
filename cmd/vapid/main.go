// Command vapid prints a new VAPID key pair for push notifications.
//
// Run once per deployment. The pair is an identity, not a secret shared with
// anybody: browsers remember the public key a subscription was made with, so
// changing it invalidates every subscription that exists.
package main

import (
	"fmt"
	"os"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func main() {
	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "vapid:", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Put these in .env. Changing them later invalidates every subscription.")
	fmt.Printf("VAPID_PUBLIC_KEY=%s\n", public)
	fmt.Printf("VAPID_PRIVATE_KEY=%s\n", private)
	fmt.Printf("VAPID_SUBJECT=mailto:you@example.com\n")
}
