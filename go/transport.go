package localai

import (
	"errors"
	"net/http"
)

// ErrEndpointRefused reports a request OA does not forward to LocalAI.
var ErrEndpointRefused = errors.New("localai: endpoint not forwarded")

// forwarded is every LocalAI endpoint OA sends to: chat completions. The
// router serves the configured model allowlist without asking LocalAI.
// Gallery (/models/apply), backend (/backends/*), settings and every other
// endpoint are refused before a connection opens (review condition 8).
var forwarded = map[string]string{
	"/v1/chat/completions": http.MethodPost,
}

// keyedTransport sends OA's per-launch API key with each forwarded request.
type keyedTransport struct {
	key  string
	next http.RoundTripper
}

func (t *keyedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if method, ok := forwarded[req.URL.Path]; !ok || method != req.Method || req.URL.RawQuery != "" {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, ErrEndpointRefused
	}
	out := req.Clone(req.Context())
	out.Header.Set("Authorization", "Bearer "+t.key)
	return t.next.RoundTrip(out)
}

func withAPIKey(client *http.Client, key string) *http.Client {
	next := client.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	keyed := *client
	keyed.Transport = &keyedTransport{key: key, next: next}
	return &keyed
}
