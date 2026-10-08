// Package notify turns a stored notification into words and sends it somewhere
// outside the application.
package notify

import (
	"fmt"

	"ticket-reservation/internal/domain"
)

// Notice is a notification already turned into words, so that every channel
// says the same thing and the wording lives in one place rather than once per
// channel.
type Notice struct {
	Subject string
	Body    string

	// Link is where it points, relative to wherever the application is served.
	Link string
}

// Render turns a notification into the words a person reads.
//
// The fallback is reachable, which is worth being honest about: the kinds are a
// closed set that ParseNotificationKind enforces on the way in, but Go does not
// check that this function covers all of them, so adding a kind and forgetting
// to write words for it compiles. The fallback is what that mistake looks like —
// a vague message rather than a blank one.
func Render(notification *domain.Notification) Notice {
	if notification.Kind == domain.NotifyEventCancelled {
		return cancelledNotice(notification)
	}

	return Notice{
		Subject: "An update about your tickets",
		Body:    "Something about your tickets has changed. Open the application to see.",
		Link:    "/",
	}
}

func cancelledNotice(notification *domain.Notification) Notice {
	subject := fmt.Sprintf("%s has been cancelled", notification.EventName)

	if notification.SeatID == "" {
		return Notice{
			Subject: subject,
			Body: fmt.Sprintf(
				"You were on the waiting list for %s. It has been cancelled, so the "+
					"queue has been cleared and there is nothing left to wait for.",
				notification.EventName),
			Link: "/",
		}
	}

	return Notice{
		Subject: subject,
		Body: fmt.Sprintf(
			"Your seat %s for %s has been cancelled. The event is no longer going ahead.",
			notification.SeatID, notification.EventName),
		Link: "/",
	}
}

// VerificationNotice is the one message that goes to an address nobody has
// verified yet, because verifying it is what it is for.
func VerificationNotice(link string) Notice {
	return Notice{
		Subject: "Confirm your email address",
		Body: "Open this link to confirm this address belongs to you:\n\n" + link +
			"\n\nIt works once, and stops working after a day. If you did not " +
			"create an account, there is nothing to do: without this nothing " +
			"else will be sent here.",
		Link: link,
	}
}
