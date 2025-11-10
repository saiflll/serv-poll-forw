package time

import (
	"IoTT/internal/config"
	"time"
)

var now = time.Now

// Now returns the current time in Asia/Jakarta timezone.
// This function can be mocked for testing.
func Now() time.Time {
	return now().In(config.Timezone)
}

// SetNow sets the function that returns the current time.
func SetNow(fn func() time.Time) {
	now = fn
}

// ResetNow resets the function that returns the current time to the default.
func ResetNow() {
	now = time.Now
}
