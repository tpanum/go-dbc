package dbc_socket_test

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/tpanum/go-dbc"
)

const (
	Username = "tpanum"
	Password = "godforgivemypasswordsins"

	LOGIN_RESP   = "{\"is_banned\": false, \"status\": 0, \"rate\": 0.139, \"balance\": 5341.051, \"user\": 275723}\n"
	UPLOAD_RESP  = "{\"status\": 0, \"captcha\": 3758452, \"is_correct\": true, \"text\": \"\"}\n"
	CAPTCHA_RESP = "{\"status\": 0, \"captcha\": 3758452, \"is_correct\": true, \"text\": \"[[101,115],[116,236],[157,236],[143,288],[91,293]]\"}\n"
)

var (
	TCPListener net.Listener
	SocketConf  = dbc.NewSocketConfig("127.0.0.1", 8081, 1)
)

type request struct {
	Cmd string
}

func setup() {
	TCPListener, _ = net.Listen("tcp", ":8081")

	go func() {
		conn, err := TCPListener.Accept()
		if err != nil {
			return
		}

		for {
			msg, _ := bufio.NewReader(conn).ReadBytes('\n')
			var r request

			json.Unmarshal(msg, &r)

			switch r.Cmd {
			case "login":
				conn.Write([]byte(LOGIN_RESP))
			case "user":
				conn.Write([]byte(LOGIN_RESP))
			case "upload":
				conn.Write([]byte(UPLOAD_RESP))
			case "captcha":
				conn.Write([]byte(CAPTCHA_RESP))
			}
		}
	}()
}

func teardown() {
	TCPListener.Close()
}

func TestMain(m *testing.M) {
	setup()

	retCode := m.Run()

	teardown()

	os.Exit(retCode)
}

func TestSocketBalance(t *testing.T) {
	client := dbc.NewSocketClient(Username, Password, SocketConf)
	balance, err := client.Balance()
	if err != nil {
		t.Errorf("Unexpected error when fetching balance: %v", err)
	}

	if balance <= 0 {
		t.Errorf("Balance incorrect (%v), expected 2645.25", balance)
	}
}

func TestSocketCaptcha(t *testing.T) {
	client := dbc.NewSocketClient(Username, Password, SocketConf)
	img, _ := ioutil.ReadFile("test.jpg")

	coords, err := client.Recaptcha(img)
	if err != nil {
		t.Errorf("Unexpected error when fetching captcha solution: %v", err)
	}

	if len(coords) <= 0 {
		t.Errorf("Coords incorrect (%v), expected one or more coordinates to be present", coords)
	}
}

func TestResize(t *testing.T) {
	img, _ := ioutil.ReadFile("big_test.png")
	newImg, err := dbc.ResizeImage(img)
	if err != nil {
		t.Errorf("Unexpected error when resizing image: %v", err)
	}

	if len(newImg) > 192000 {
		t.Errorf("Expected new image size to be less than 192kb")
	}
}
