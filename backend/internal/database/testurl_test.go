package database

import (
	"strings"
	"testing"
)

const devURL = "postgres://switchyard:switchyard@localhost:5434/switchyard"

func TestCheckTestURLRefusesTheApplicationsDatabase(t *testing.T) {
	// Every one of these is the development database wearing a different hat.
	// The first is the exact string somebody pastes; the rest are what it looks
	// like after being retyped from memory.
	sameDatabase := []string{
		devURL,
		"postgres://switchyard:switchyard@127.0.0.1:5434/switchyard",
		"postgres://switchyard:switchyard@LOCALHOST:5434/switchyard",
		"postgres://switchyard:switchyard@localhost:5434/switchyard?sslmode=disable",
		"postgres://other:creds@localhost:5434/switchyard",
	}

	for _, testURL := range sameDatabase {
		err := CheckTestURL(testURL, devURL)
		if err == nil {
			t.Errorf("CheckTestURL(%q) allowed the application's own database", testURL)
			continue
		}
		// The refusal names the database and never the password.
		if strings.Contains(err.Error(), "switchyard:switchyard") {
			t.Errorf("the refusal leaked credentials: %v", err)
		}
	}
}

func TestCheckTestURLAllowsAThrowawayDatabase(t *testing.T) {
	safe := []string{
		// A different port is the documented setup.
		"postgres://postgres:t@localhost:55433/switchyard",
		// A different database on the same server.
		"postgres://switchyard:switchyard@localhost:5434/switchyard_test",
		// A different host.
		"postgres://switchyard:switchyard@db.example.com:5434/switchyard",
	}

	for _, testURL := range safe {
		if err := CheckTestURL(testURL, devURL); err != nil {
			t.Errorf("CheckTestURL(%q) refused a separate database: %v", testURL, err)
		}
	}
}

// Nothing set means nothing to protect, and nothing to test either.
func TestCheckTestURLIsQuietWhenThereIsNothingToCompare(t *testing.T) {
	if err := CheckTestURL(devURL, ""); err != nil {
		t.Errorf("with no DATABASE_URL: %v", err)
	}
	if err := CheckTestURL("", devURL); err != nil {
		t.Errorf("with no test URL: %v", err)
	}
}

// A URL that will not parse must not be waved through by accident. It cannot be
// compared properly, so it falls back to the strict answer.
func TestCheckTestURLIsStrictAboutUnparseableURLs(t *testing.T) {
	if err := CheckTestURL("://nonsense", "://nonsense"); err == nil {
		t.Error("two identical unparseable URLs were allowed")
	}
}
