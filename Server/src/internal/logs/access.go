package logs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/weirdstreamer/server/internal/clientip"
)

type accessKey struct{}

// request is what the access log learns about a request while it's handled.
type request struct {
	user string
}

// SetUser names the signed-in user of r in its access log line (the auth middleware knows it,
// the access log's middleware runs outside it).
func SetUser(r *http.Request, user string) {
	if req, ok := r.Context().Value(accessKey{}).(*request); ok {
		req.user = user
	}
}

// Access logs each request when it's done, in the combined log format plus how long it took: a
// stream's line comes when the player lets go of it.
func Access(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if accessLog == nil {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		req := &request{user: "-"}
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), accessKey{}, req)))
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		host := clientip.Of(r)
		fmt.Fprintf(accessLog, "%s - %s [%s] %q %d %d %q %q %dms\n",
			host, req.user, start.Format("02/Jan/2006:15:04:05 -0700"),
			r.Method+" "+r.URL.RequestURI()+" "+r.Proto, rec.status, rec.bytes,
			orDash(r.Referer()), orDash(r.UserAgent()), time.Since(start).Milliseconds())
	})
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// recorder notes a response's status and size. Streams flush as they go, so it passes Flush on,
// and Unwrap lets http.ResponseController reach the real writer.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += int64(n)
	return n, err
}

func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
