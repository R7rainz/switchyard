package database

import (
	"fmt"
	"net/url"
	"strings"
)

// CheckTestURL reports whether a database is safe for the integration tests to
// reset, given the URL the application itself uses.
//
// The tests run `drop schema public cascade`. Pointed at the development
// database that is every workflow, execution, and user gone — and the two URLs
// are similar enough to paste one where the other belongs, which is exactly how
// it happens. This is the check that makes that a failed test rather than a
// restore from a backup nobody took.
//
// Both arguments are passed in rather than read from the environment, because
// config is the only package that reads env vars and a test helper is no reason
// to make it two. An empty protected URL means there is nothing to protect.
func CheckTestURL(testURL, protectedURL string) error {
	if protectedURL == "" || testURL == "" {
		return nil
	}
	if !sameDatabase(testURL, protectedURL) {
		return nil
	}
	return fmt.Errorf(
		"database: SWITCHYARD_TEST_DATABASE_URL points at the same database as DATABASE_URL (%s); "+
			"these tests drop the public schema, so point them at a throwaway database instead",
		describe(testURL))
}

// sameDatabase compares where two URLs land rather than how they are spelled.
// The same database is reachable as localhost and 127.0.0.1, with and without
// an explicit port, with and without query parameters — a string comparison
// would wave all of those through.
func sameDatabase(a, b string) bool {
	hostA, nameA, okA := target(a)
	hostB, nameB, okB := target(b)
	if !okA || !okB {
		// Something unparseable. Fall back to the strict answer rather than
		// deciding two URLs differ because one of them is malformed.
		return a == b
	}
	return hostA == hostB && nameA == nameB
}

// target reduces a URL to the host:port and database name it resolves to.
func target(raw string) (host, name string, ok bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", "", false
	}

	hostname := strings.ToLower(parsed.Hostname())
	// Not a DNS lookup — just the one alias that is certain and is what a
	// hand-edited connection string actually differs by.
	if hostname == "localhost" {
		hostname = "127.0.0.1"
	}

	port := parsed.Port()
	if port == "" {
		port = "5432"
	}

	return hostname + ":" + port, strings.TrimPrefix(parsed.Path, "/"), true
}

// describe is the URL without its credentials, so a failure can name the
// database it refused without printing a password.
func describe(raw string) string {
	host, name, ok := target(raw)
	if !ok {
		return "unparseable URL"
	}
	return host + "/" + name
}
