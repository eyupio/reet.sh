package privateconn

import (
	"context"
	"net/http"
)

type transportContextKey struct{}

// WithPrivateTransport records that a request reached the application through
// the authenticated Tailcat listener. Only the private listener adapter may
// call it. Never derive transport trust from a request header, URL, or agent
// message field.
func WithPrivateTransport(ctx context.Context) context.Context {
	return context.WithValue(ctx, transportContextKey{}, true)
}

// IsPrivateTransport reports whether the listener marked this request.
func IsPrivateTransport(ctx context.Context) bool {
	marked, _ := ctx.Value(transportContextKey{}).(bool)
	return marked
}

func markTransport(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithPrivateTransport(r.Context())))
	})
}
