package auth

import (
	"sync"
	"time"
)

// Limiter slows down password guessing. Each key (a username or a client address) that gets
// freeAttempts wrong passwords in a row is locked out for baseDelay, doubling with each further
// failure up to maxDelay. A right password clears the username's failures (not the address's,
// so signing in to one account doesn't reset guesses at others), and a key with no failure for
// maxDelay starts over.
//
// Checks still running count against the limit, so parallel guesses can't all start before the
// first one fails. An address the user signed in from is exempt from the username's lock, so
// someone guessing at a name can't lock its owner out of the devices they already use.
type Limiter struct {
	mu        sync.Mutex
	now       func() time.Time
	failures  map[string]*failureRecord
	known     map[knownAddress]time.Time // last success from each user's address
	lastSweep time.Time
}

type failureRecord struct {
	count       int
	pending     int // checks begun and not yet finished
	last        time.Time
	lockedUntil time.Time
}

type knownAddress struct{ username, addr string }

const (
	freeAttempts = 5
	baseDelay    = 30 * time.Second
	maxDelay     = 60 * time.Minute
	// knownFor is how long an address stays exempt from its user's lock after a success there.
	knownFor = 30 * 24 * time.Hour
	// busyWait is the wait given to a check refused because another one for the key is running.
	busyWait = time.Second
)

func NewLimiter() *Limiter {
	return &Limiter{
		now:      time.Now,
		failures: make(map[string]*failureRecord),
		known:    make(map[knownAddress]time.Time),
	}
}

// Attempt is one password check let through by Begin. Exactly one of Failed, Passed or Abandon
// must be called when the check is done.
type Attempt struct {
	l        *Limiter
	username string
	addr     string
	keys     []string
}

// Begin starts a password check of username from the client address addr. It returns how long
// to wait instead when a lock is in force on either, or when the remaining free attempts are
// already taken by checks still running.
func (l *Limiter) Begin(username, addr string) (*Attempt, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	keys := []string{"ip:" + addr}
	if seen, ok := l.known[knownAddress{username, addr}]; !ok || now.Sub(seen) > knownFor {
		keys = append(keys, "user:"+username)
	}

	var wait time.Duration
	for _, key := range keys {
		rec := l.failures[key]
		if rec == nil {
			continue
		}
		if rec.pending == 0 && now.Sub(rec.last) > maxDelay {
			delete(l.failures, key)
			continue
		}
		if left := rec.lockedUntil.Sub(now); left > wait {
			wait = left
		}
		// Past the free attempts (or once locked), one check at a time.
		if rec.pending > 0 && rec.count+rec.pending >= freeAttempts && busyWait > wait {
			wait = busyWait
		}
	}
	if wait > 0 {
		return nil, wait
	}
	for _, key := range keys {
		rec := l.failures[key]
		if rec == nil {
			rec = &failureRecord{last: now}
			l.failures[key] = rec
		}
		rec.pending++
	}
	return &Attempt{l: l, username: username, addr: addr, keys: keys}, 0
}

// Failed records a wrong password.
func (a *Attempt) Failed() {
	l := a.l
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for _, key := range a.keys {
		rec := l.failures[key]
		rec.pending--
		rec.count++
		rec.last = now
		if delay := delayFor(rec.count); delay > 0 {
			rec.lockedUntil = now.Add(delay)
		}
	}
}

// Passed records a right password: the username's failures are cleared and the address becomes
// known for the user.
func (a *Attempt) Passed() {
	l := a.l
	l.mu.Lock()
	defer l.mu.Unlock()
	l.release(a.keys)
	delete(l.failures, "user:"+a.username)
	l.known[knownAddress{a.username, a.addr}] = l.now()
}

// Abandon ends a check that couldn't be made (a server error), counting it neither way.
func (a *Attempt) Abandon() {
	a.l.mu.Lock()
	defer a.l.mu.Unlock()
	a.l.release(a.keys)
}

// release ends the running checks on keys. Callers hold l.mu.
func (l *Limiter) release(keys []string) {
	for _, key := range keys {
		rec := l.failures[key]
		rec.pending--
		if rec.pending == 0 && rec.count == 0 {
			delete(l.failures, key)
		}
	}
}

// sweep forgets keys that have been quiet for maxDelay and addresses unused for knownFor, so the
// maps don't grow with every address and name ever guessed. It runs at most once a minute.
// Callers hold l.mu.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for key, rec := range l.failures {
		if rec.pending == 0 && now.Sub(rec.last) > maxDelay {
			delete(l.failures, key)
		}
	}
	for k, seen := range l.known {
		if now.Sub(seen) > knownFor {
			delete(l.known, k)
		}
	}
}

// delayFor is the lock after the given number of failures in a row: none until freeAttempts,
// then baseDelay doubling up to maxDelay.
func delayFor(failures int) time.Duration {
	if failures < freeAttempts {
		return 0
	}
	n := failures - freeAttempts
	if n >= 10 {
		return maxDelay
	}
	if delay := baseDelay << n; delay < maxDelay {
		return delay
	}
	return maxDelay
}
