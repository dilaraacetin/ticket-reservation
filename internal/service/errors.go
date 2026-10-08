package service

import "errors"

var ErrInvalidCredentials = errors.New("the email address or password is wrong")

// ErrLogoutUnavailable means nothing is wired in to remember a signed out token,
// so the service would be claiming to have done something it cannot.
var ErrLogoutUnavailable = errors.New("signing out is not available")
