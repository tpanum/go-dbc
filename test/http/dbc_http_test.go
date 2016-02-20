package dbc_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tpanum/go-dbc"
)

const (
	Username = "tpanum"
	Password = "godforgivemypasswordsins"
)

var (
	HTTPServer  *httptest.Server
	HTTPConf    dbc.HTTPConfig
	TCPListener net.Listener
)

func setup() {
	HTTPServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/user":
			fmt.Fprintln(w, `{ "user": 264, "rate": 0.02, "balance" : 2645.25 }`)
		}
	}))

	HTTPConf = dbc.HTTPConfig{HTTPServer.URL, 5}

	TCPListener, _ = net.Listen("tcp", ":8081")
}

func teardown() {
	HTTPServer.Close()
}

func TestMain(m *testing.M) {
	setup()

	retCode := m.Run()

	teardown()

	os.Exit(retCode)
}

func TestHTTPBalance(t *testing.T) {
	client := dbc.NewHTTPClient(Username, Password, HTTPConf)
	balance, err := client.Balance()
	if err != nil {
		t.Errorf("Unexpected error when fetching balance: %v", err)
	}

	if balance <= 0 {
		t.Errorf("Balance incorrect (%v), expected 2645.25", balance)
	}
}
