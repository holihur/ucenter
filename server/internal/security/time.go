package security

import "time"

var timeNow = time.Now

// SetTimeNow is used by tests to control TOTP validation time.
func SetTimeNow(fn func() time.Time) { timeNow = fn }
