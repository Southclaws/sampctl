package download

import "net/http"

// HTTPDoer lets us test HTTP clients
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}
