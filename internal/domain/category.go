package domain

import "fmt"

// Category is the kind of thing an event is, and the catalogue's main way of
// narrowing itself.
//
// A closed set rather than free text, for the reason a filter exists at all: one
// venue typing "Stand-up" and another "stand up" would split the same category
// into two that each find half the events, and nothing would say so.
type Category string

const (
	CategoryConcert  Category = "concert"
	CategoryTheatre  Category = "theatre"
	CategoryComedy   Category = "comedy"
	CategoryFestival Category = "festival"
	CategorySport    Category = "sport"
	CategoryFamily   Category = "family"
	CategoryOther    Category = "other"
)

// DefaultCategory is what an event falls into when nothing better fits, and what
// events stored before the catalogue had categories were given.
const DefaultCategory = CategoryOther

// Categories is every category, in the order a list of them should be offered.
// Other goes last: it is the fallback, not a choice to lead with.
var Categories = []Category{
	CategoryConcert,
	CategoryTheatre,
	CategoryComedy,
	CategoryFestival,
	CategorySport,
	CategoryFamily,
	CategoryOther,
}

// ParseCategory turns a stored or submitted value into a category and refuses
// anything else, so a typo cannot become a category nothing will ever match.
func ParseCategory(value string) (Category, error) {
	for _, category := range Categories {
		if Category(value) == category {
			return category, nil
		}
	}

	return "", fmt.Errorf("%w: %q", ErrUnknownCategory, value)
}

func (c Category) String() string {
	return string(c)
}
