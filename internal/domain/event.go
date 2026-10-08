package domain

import (
	"fmt"
	"net/url"
	"time"
)

// Event is something people buy seats for: a concert, a screening, a departure.
// Seats are stored per event by the repository rather than embedded here,
// because a hall can hold thousands of them and almost every operation touches
// exactly one.
type Event struct {
	ID       string
	Name     string
	Venue    string
	StartsAt time.Time

	// Details is what a visitor reads to decide. Kept as one value so that the
	// calls carrying an event around do not grow a parameter every time the
	// catalogue learns another descriptive field.
	Details EventDetails

	// CancelledAt is zero while the event is on sale. Withdrawing an event marks
	// it rather than removing it, because the people already holding tickets to
	// it need to be able to see what happened.
	CancelledAt time.Time
}

// EventDetails is the descriptive half of an event. Nothing here takes part in
// deciding whether a seat may be sold; it exists so somebody browsing has
// enough to go on before they open the seat map.
type EventDetails struct {
	// City is what the catalogue filters on, so it is held apart from Venue: a
	// hall name is not something a visitor can narrow a list by.
	City string

	// Category is the other thing it filters on. Empty is read as the default
	// rather than refused, because an event that nobody classified is still an
	// event and "other" is what it is.
	Category Category

	// ImageURL is the poster. It is rendered as an image source, so only the two
	// schemes that mean "fetch a picture over the network" are accepted.
	ImageURL string

	Description string
	Rules       string
}

// Field limits. Long enough for a real description, short enough that one
// request cannot park an essay in the catalogue.
const (
	MaxEventTextLength = 4000
	MaxImageURLLength  = 600
)

// WithDefaults fills in what was left out. Called where details are stored, so
// a category read back from the catalogue is never the empty string.
func (d EventDetails) WithDefaults() EventDetails {
	if d.Category == "" {
		d.Category = DefaultCategory
	}

	return d
}

// Validate reports whether the details may be stored. Callers apply WithDefaults
// first, so an absent category is the default by the time it is checked here
// rather than a second thing every check has to allow for.
//
// Only City is required beyond that: a poster, a description and a house rule
// are each things a venue may genuinely not have yet, and refusing the event
// over one would be refusing it over nothing.
func (d EventDetails) Validate() error {
	if d.City == "" {
		return fmt.Errorf("%w: the city is empty", ErrInvalidEvent)
	}

	if _, err := ParseCategory(string(d.Category)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
	}

	for label, text := range map[string]string{"description": d.Description, "rules": d.Rules} {
		if len(text) > MaxEventTextLength {
			return fmt.Errorf("%w: the %s is longer than %d characters",
				ErrInvalidEvent, label, MaxEventTextLength)
		}
	}

	return d.validateImage()
}

func (d EventDetails) validateImage() error {
	if d.ImageURL == "" {
		return nil
	}

	if len(d.ImageURL) > MaxImageURLLength {
		return fmt.Errorf("%w: the image address is longer than %d characters",
			ErrInvalidEvent, MaxImageURLLength)
	}

	parsed, err := url.Parse(d.ImageURL)
	if err != nil {
		return fmt.Errorf("%w: the image address is not a URL", ErrInvalidEvent)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: the image address must be http or https, got %q",
			ErrInvalidEvent, parsed.Scheme)
	}

	if parsed.Host == "" {
		return fmt.Errorf("%w: the image address has no host", ErrInvalidEvent)
	}

	return nil
}

// IsCancelled reports whether the event has been withdrawn from sale.
func (e *Event) IsCancelled() bool {
	return !e.CancelledAt.IsZero()
}

// Cancel withdraws the event from sale.
//
// No new seats may be claimed afterwards, but anyone already holding one may
// still confirm it: they were part way through a purchase when this happened,
// and taking it away from them is a worse answer than letting them finish.
func (e *Event) Cancel(now time.Time) error {
	if e.IsCancelled() {
		return ErrEventAlreadyCancelled
	}

	e.CancelledAt = now.UTC().Truncate(time.Second)

	return nil
}

// Update changes the parts of an event that are safe to change. Empty values and
// a zero time mean "leave this alone", so a caller can send one field without
// having to repeat the others.
//
// A cancelled event is not editable: it is a record of something that is not
// happening, and editing it would quietly turn it into a different one.
func (e *Event) Update(name, venue string, startsAt time.Time, details EventDetails) error {
	if e.IsCancelled() {
		return ErrEventAlreadyCancelled
	}

	details = details.WithDefaults()
	if err := details.Validate(); err != nil {
		return err
	}

	if name != "" {
		e.Name = name
	}

	if venue != "" {
		e.Venue = venue
	}

	if !startsAt.IsZero() {
		e.StartsAt = startsAt.UTC().Truncate(time.Second)
	}

	// Taken as written rather than treated the way the fields above are. A name
	// may not be blank, so a blank one can stand for "unchanged"; a description
	// may be blank, so the same trick would make one impossible to clear.
	e.Details = details

	return nil
}

// HasStarted reports whether the event has already begun at now. Whether a
// started event still sells is a policy call, so it is left to the service
// layer.
func (e *Event) HasStarted(now time.Time) bool {
	return reached(e.StartsAt, now)
}
