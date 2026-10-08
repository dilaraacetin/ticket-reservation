package domain

import "fmt"

// Role is what an account is allowed to do.
//
// A closed set on purpose. A role nobody has written a check for is a role that
// grants nothing, which is the safe direction; a free-form string would instead
// let a typo like "admn" look like a role and silently grant none of it while
// reading as if it had.
type Role string

const (
	RoleCustomer Role = "customer"
	RoleAdmin    Role = "admin"
)

// DefaultRole is what a new account gets. Nobody becomes an administrator by
// signing up.
const DefaultRole = RoleCustomer

// ParseRole turns a stored value back into a role and refuses anything else. A
// row that somehow holds "superuser" has to fail rather than be read as one.
func ParseRole(value string) (Role, error) {
	switch role := Role(value); role {
	case RoleCustomer, RoleAdmin:
		return role, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownRole, value)
	}
}

func (r Role) String() string {
	return string(r)
}

// IsAdmin is the single question the authorization middleware asks, so that
// comparing against a role constant is not spread across handlers.
func (r Role) IsAdmin() bool {
	return r == RoleAdmin
}
