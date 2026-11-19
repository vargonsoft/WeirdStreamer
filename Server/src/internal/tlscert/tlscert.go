// Package tlscert watches the server's HTTPS certificate for its expiry: a warning in the log
// from warnDays before it runs out, an error once it has, and the same for admins in Settings
// (Current, through GET /api/admin/certificate).
package tlscert

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"
)

// warnDays is how long before the certificate expires the warnings start.
const warnDays = 30

// Status is the certificate's state, as Settings shows it.
type Status struct {
	NotAfter    time.Time `json:"not_after"`
	DaysLeft    int       `json:"days_left"`               // calendar days to expiry; negative once expired
	State       string    `json:"state"`                   // "ok", "expiring" or "expired"
	NewerOnDisk bool      `json:"newer_on_disk,omitempty"` // certFile now holds a later one: a restart serves it
	Message     string    `json:"message,omitempty"`       // for people, when there's something to do
}

var current atomic.Pointer[Status]

// Current is the last check's result, nil when the server isn't serving HTTPS.
func Current() *Status { return current.Load() }

// NotAfter reads when the first certificate in a PEM file expires.
func NotAfter(certFile string) (time.Time, error) {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return time.Time{}, err
	}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return time.Time{}, errors.New("no certificate in " + certFile)
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return time.Time{}, err
			}
			return cert.NotAfter, nil
		}
	}
}

// Check works out the status of a served certificate that expires at served, with certFile's
// own expiry (zero if unreadable), at now.
func Check(served, onDisk, now time.Time) Status {
	s := Status{NotAfter: served, DaysLeft: calendarDays(now, served), State: "ok"}
	day := served.Local().Format("2 January 2006")
	when := fmt.Sprintf("in %d days", s.DaysLeft)
	switch s.DaysLeft {
	case 0:
		when = "today"
	case 1:
		when = "tomorrow"
	}
	const replacing = " Replacing it: the Android and iPhone apps will refuse the new certificate until each device removes the server and adds it again."
	switch {
	case !now.Before(served):
		s.State = "expired"
		s.Message = fmt.Sprintf("The server's HTTPS certificate expired on %s. Browsers warn about it; put a new one in server.certFile and keyFile, then restart the server.%s", day, replacing)
	case s.DaysLeft < warnDays:
		s.State = "expiring"
		s.Message = fmt.Sprintf("The server's HTTPS certificate expires on %s (%s). Put a new one in server.certFile and keyFile, then restart the server.%s", day, when, replacing)
	}
	if onDisk.After(served) {
		s.NewerOnDisk = true
		s.Message = fmt.Sprintf("server.certFile holds a newer certificate (until %s) than the one in use: restart the server to use it.%s",
			onDisk.Local().Format("2 January 2006"), replacing)
	}
	return s
}

// calendarDays counts the days from from's date to to's, on the server's calendar.
func calendarDays(from, to time.Time) int {
	y1, m1, d1 := from.Local().Date()
	y2, m2, d2 := to.Local().Date()
	return int(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
}

// Watch checks the served certificate (expiring at served) now and once a day until ctx ends,
// logging when it's close to expiring or has, or certFile holds a newer one.
func Watch(ctx context.Context, certFile string, served time.Time) {
	check := func() {
		onDisk, _ := NotAfter(certFile)
		s := Check(served, onDisk, time.Now())
		current.Store(&s)
		switch {
		case s.State == "expired":
			log.Printf("[TLS] Error: %s", s.Message)
		case s.State == "expiring" || s.NewerOnDisk:
			log.Printf("[TLS] Warning: %s", s.Message)
		}
	}
	check()
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				check()
			}
		}
	}()
}
