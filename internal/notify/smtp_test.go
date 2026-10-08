package notify

import (
	"strings"
	"testing"

	"ticket-reservation/internal/domain"
)

func testMailer(t *testing.T) *Mailer {
	t.Helper()

	mailer, err := NewMailer(MailerConfig{Addr: "localhost:1025", From: "tickets@example.com"})
	if err != nil {
		t.Fatalf("NewMailer() error = %v", err)
	}

	return mailer
}

// A newline in a header value would end that header and let whatever follows be
// read as another one, which is how a subject becomes a Bcc.
func TestMailer_RefusesToFoldAHeader(t *testing.T) {
	message := string(testMailer(t).compose("someone@example.com", Notice{
		Subject: "Cancelled\r\nBcc: everyone@example.com",
		Body:    "nothing to see",
	}))

	headers, _, found := strings.Cut(message, "\r\n\r\n")
	if !found {
		t.Fatal("the message has no header section")
	}

	// A line of its own is what makes something a header. The words may still
	// appear inside the subject's value, which is harmless.
	for line := range strings.SplitSeq(headers, "\r\n") {
		name, _, isHeader := strings.Cut(line, ":")
		if !isHeader {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(name)) {
		case "from", "to", "subject", "date", "mime-version", "content-type":
		default:
			t.Errorf("a header was injected through the subject: %q", line)
		}
	}

	// And the subject is still one line.
	if lines := strings.Count(headers, "Subject:"); lines != 1 {
		t.Errorf("found %d subject headers, want 1", lines)
	}
}

// An event called "Fazıl Say" is the ordinary case here, and a subject that is
// not encoded arrives as mojibake.
func TestMailer_EncodesANonASCIISubject(t *testing.T) {
	message := string(testMailer(t).compose("someone@example.com", Notice{
		Subject: "Fazıl Say has been cancelled",
		Body:    "nothing to see",
	}))

	if strings.Contains(message, "Subject: Fazıl") {
		t.Error("the subject was sent raw, so it will arrive as mojibake")
	}

	if !strings.Contains(message, "Subject: =?utf-8?") {
		t.Errorf("the subject is not encoded:\n%s", message)
	}
}

func TestMailer_IsOffWithoutAServer(t *testing.T) {
	mailer, err := NewMailer(MailerConfig{})
	if err != nil {
		t.Fatalf("NewMailer() error = %v", err)
	}
	if mailer != nil {
		t.Error("a mailer was built with no server to send to")
	}

	// And a nil mailer says so rather than panicking.
	if err := mailer.SendEmail(t.Context(), "someone@example.com", Notice{}); err == nil {
		t.Error("SendEmail() on a mailer that is off returned no error")
	}
}

func TestMailer_RefusesABadFromAddress(t *testing.T) {
	if _, err := NewMailer(MailerConfig{Addr: "localhost:1025", From: "not an address"}); err == nil {
		t.Error("NewMailer() accepted a from address that is not one")
	}
}

// Every channel says the same thing, and the two shapes of the cancellation
// notice are different messages: one is about a seat, the other about a place in
// a queue.
func TestRender_CancellationReadsForBothCases(t *testing.T) {
	withSeat := Render(&domain.Notification{
		Kind: domain.NotifyEventCancelled, EventName: "Fazil Say", SeatID: "A1",
	})

	if !strings.Contains(withSeat.Body, "A1") || !strings.Contains(withSeat.Body, "Fazil Say") {
		t.Errorf("the seat notice does not say which seat or which event: %q", withSeat.Body)
	}

	waiting := Render(&domain.Notification{
		Kind: domain.NotifyEventCancelled, EventName: "Fazil Say",
	})

	if strings.Contains(waiting.Body, "seat") {
		t.Errorf("the waiting list notice talks about a seat the person never had: %q", waiting.Body)
	}
	if !strings.Contains(waiting.Body, "waiting list") {
		t.Errorf("the waiting list notice does not say what it is about: %q", waiting.Body)
	}

	if withSeat.Subject == "" || waiting.Subject == "" {
		t.Error("a notice with no subject")
	}
}

// The fallback is reachable: Go does not check that Render covers every kind, so
// adding one and forgetting the words compiles. This is what that looks like.
func TestRender_FallsBackRatherThanSendingNothing(t *testing.T) {
	notice := Render(&domain.Notification{Kind: "something_nobody_wrote_words_for"})

	if notice.Subject == "" || notice.Body == "" {
		t.Error("a kind with no words produced an empty message")
	}
}
