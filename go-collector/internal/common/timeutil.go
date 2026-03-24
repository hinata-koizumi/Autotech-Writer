package common

import "time"

// ShouldFetchArxivNow returns true if the current JST time is within the arXiv
// peak window (09:00–11:00 JST), when arXiv publishes new papers.
// It always uses Asia/Tokyo regardless of the server's local timezone.
func ShouldFetchArxivNow(now time.Time) bool {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		// Fallback: UTC+9
		jst = time.FixedZone("JST", 9*60*60)
	}
	jstNow := now.In(jst)
	hour := jstNow.Hour()
	return hour >= 9 && hour < 11
}
