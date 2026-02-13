package auth

import "time"

// Clock abstracts time for testability.
type Clock interface { // A
	Now() time.Time
}

type realClock struct{} // A

// Now returns the current time.
func (realClock) Now() time.Time { // A
	return time.Now()
}
