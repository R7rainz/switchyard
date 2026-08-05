package execution

import (
	"testing"
	"time"
)

func TestCronMatchesExactAndWildcardMinutes(t *testing.T) {
	now := time.Date(2026, time.August, 5, 9, 30, 0, 0, time.UTC)
	if !cronMatches("30 9 * * *", now) {
		t.Fatal("exact schedule did not match")
	}
	if cronMatches("31 9 * * *", now) {
		t.Fatal("wrong minute matched")
	}
	if !cronMatches("* * * * *", now) {
		t.Fatal("wildcard schedule did not match")
	}
	if !cronMatches("*/15 9 * * 1-5", now) {
		t.Fatal("weekday range schedule did not match")
	}
	if cronMatches("30 9 * * 0", now) {
		t.Fatal("Sunday schedule matched a Wednesday")
	}
	if !cronMatches("30 9 5,6,7 * *", now) {
		t.Fatal("day list schedule did not match")
	}
}
