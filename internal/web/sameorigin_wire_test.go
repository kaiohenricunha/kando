package web

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestSameOriginOverARealConnection drives the guard through a real
// net/http.Server and a real TCP connection, which TestSameOriginMiddleware
// does not: that test builds requests with httptest.NewRequest and sets
// req.Host directly, so it covers the guard's decision table but never the
// server's own parsing of the Host header and the request line.
//
// That distinction is the point. sameOrigin is the whole authorization story
// for kando web — there is no login, so Host, Origin and Sec-Fetch-Site are
// the entire boundary — and it is decided from values net/http parses off the
// wire. A toolchain bump can change that parsing underneath an in-process
// test that never exercises it, which is exactly what happened to prompt this
// (the Go 1.25 -> 1.27 move in #16 shifts program-wide GODEBUG defaults, and
// net/http request-parsing changes have historically shipped that way).
//
// So this asserts the outcomes that matter end to end rather than restating
// the decision table: a forged Origin, a cross-site fetch and a foreign Host
// are refused over the wire, and the legitimate browser form post still gets
// through.
func TestSameOriginOverARealConnection(t *testing.T) {
	root := newRoot(t)
	ln, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, ln, Options{Root: root, Board: "life", Now: func() time.Time { return fixedNow }})
	}()

	// A client that never follows redirects, so a 303 is not mistaken for the
	// guard having allowed something it did not.
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	cases := []struct {
		name    string
		host    string // overrides the Host header when set
		headers []string
		want    int
	}{
		{
			name: "plain request from the bound address",
			want: 200,
		},
		{
			// The shape a real browser form post takes against this app:
			// secureHeaders sets Referrer-Policy: no-referrer, so Chrome sends
			// Origin: null, and Sec-Fetch-Site says it is same-origin.
			name:    "browser form post with a withheld origin",
			headers: []string{"Origin", "null", "Sec-Fetch-Site", "same-origin"},
			want:    200,
		},
		{
			name:    "forged origin",
			headers: []string{"Origin", "http://evil.example"},
			want:    http.StatusForbidden,
		},
		{
			name:    "cross-site fetch",
			headers: []string{"Sec-Fetch-Site", "cross-site"},
			want:    http.StatusForbidden,
		},
		{
			// DNS rebinding: the request reaches the loopback listener, but
			// the Host names somewhere else. This is the case that depends
			// most directly on how net/http parses Host off the wire.
			name: "foreign host header",
			host: "evil.example",
			want: http.StatusForbidden,
		},
		{
			// A Host that is loopback but on the wrong port is still not this
			// origin, and the port comes from the same parse.
			name: "right host, wrong port",
			host: "127.0.0.1:1",
			want: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/b/life", nil)
			if err != nil {
				t.Fatal(err)
			}
			// req.Host overrides the Host header on the wire while the
			// connection still goes to the real listener.
			if tc.host != "" {
				req.Host = tc.host
			}
			for i := 0; i+1 < len(tc.headers); i += 2 {
				req.Header.Set(tc.headers[i], tc.headers[i+1])
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
