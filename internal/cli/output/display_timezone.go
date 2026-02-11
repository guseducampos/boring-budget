package output

import "sync/atomic"

var displayTimezone atomic.Value

func init() {
	displayTimezone.Store("UTC")
}

func SetDisplayTimezone(timezone string) {
	if timezone == "" {
		displayTimezone.Store("UTC")
		return
	}
	displayTimezone.Store(timezone)
}

func CurrentDisplayTimezone() string {
	value, _ := displayTimezone.Load().(string)
	if value == "" {
		return "UTC"
	}
	return value
}
