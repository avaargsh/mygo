package xerj

import (
	"net/http"
	"testing"
)

type privateDefaultTransport struct{}

func (privateDefaultTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("XERJ must not use the HTTP default transport")
}

func TestClientDoesNotAssumeDefaultTransportType(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = privateDefaultTransport{}
	t.Cleanup(func() { http.DefaultTransport = old })
	c, err := New("http://127.0.0.1:9200", "local")
	if err != nil {
		t.Fatal(err)
	}
	c.CloseIdleConnections()
}
