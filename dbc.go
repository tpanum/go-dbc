package dbc

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"encoding/base64"
	"encoding/json"

	"github.com/nfnt/resize"
)

const (
	API_VERSION = "go-dbc/1.0.0"

	DEFAULT_TIMEOUT = 60
	POLLS_INTERVAL  = 5

	HTTP_BASE_URL      = "http://api.dbcapi.me/api"
	HTTP_RESPONSE_TYPE = "application/json"

	SOCKET_HOST      = "api.dbcapi.me"
	MAXIMUM_IMG_SIZE = 143000
)

var (
	SOCKET_PORTS = []int{8123, 8124, 8125, 8126, 8127, 8128, 8129, 8130, 8131}
)

type DBCClient interface {
	Balance() (float64, error)
}

type Client struct {
	Username string
	Password string
	Verbose  bool
}

type HTTPConfig struct {
	URL          string
	PollInterval time.Duration
}

type HTTPClient struct {
	Client
	Config HTTPConfig
}

func NewHTTPClient(username, password string, conf HTTPConfig) *HTTPClient {
	return &HTTPClient{Client{Username: username, Password: password}, conf}
}

func (c *HTTPClient) Call(cmd string, payload url.Values) (map[string]interface{}, error) {
	url := c.Config.URL + "/" + strings.Trim(cmd, "/")

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", HTTP_RESPONSE_TYPE)
	req.Header.Set("User-Agent", API_VERSION)
	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 403:
		return nil, fmt.Errorf("Access denied, please check your credentials and/or balance")
	case 400:
		return nil, fmt.Errorf("CAPTCHA was rejected by the service, check if it's a valid image")
	case 413:
		return nil, fmt.Errorf("CAPTCHA was rejected by the service, check if it's a valid image")
	case 503:
		return nil, fmt.Errorf("CAPTCHA was rejected due to service overload, try again later")
	}

	m := make(map[string]interface{})
	err = json.NewDecoder(resp.Body).Decode(&m)
	if err != nil {
		return nil, fmt.Errorf("Invalid API Response")
	}

	return m, nil
}

func (c *HTTPClient) UserURL() url.Values {
	return url.Values{
		"username": {c.Username},
		"password": {c.Password},
	}
}

func (c *HTTPClient) Balance() (float64, error) {
	json, err := c.Call("user", c.UserURL())
	if err != nil {
		return 0, err
	}

	balance, ok := json["balance"].(float64)
	if !ok {
		return 0, fmt.Errorf("Balance is not a float")
	}

	return balance, nil
}

type SocketConfig struct {
	url          string
	port         int
	pollInterval time.Duration
}

func NewSocketConfig(url string, port int, interval time.Duration) *SocketConfig {
	return &SocketConfig{url, port, interval}
}

func (sc SocketConfig) URL() string {
	if sc.url != "" {
		return sc.url
	}

	return SOCKET_HOST
}

func (sc SocketConfig) Port() int {
	if sc.port != 0 {
		return sc.port
	}

	return SOCKET_PORTS[rand.Intn(len(SOCKET_PORTS))]
}

func (sc SocketConfig) PollInterval() time.Duration {
	if sc.pollInterval != 0 {
		return sc.pollInterval
	}

	return POLLS_INTERVAL
}

type SocketClient struct {
	Client
	Config     *SocketConfig
	Connection net.Conn
}

type UserResp struct {
	Balance  float64
	IsBanned bool `json:"is_banned"`
	Status   int
	Rate     float64
}

type RCoord struct {
	X float64
	Y float64
}

func NewSocketClient(username, password string, conf *SocketConfig) *SocketClient {
	userinfo := Client{Username: username, Password: password}

	return &SocketClient{Client: userinfo, Config: conf}
}

func (c *SocketClient) Call(cmd string, data map[string]interface{}, target interface{}) error {
	var err error

	if data == nil {
		data = make(map[string]interface{})
	}

	if c.Connection == nil {
		c.Connection, err = net.Dial("tcp", fmt.Sprintf("%v:%v", c.Config.URL(), c.Config.Port()))
		if err != nil {
			return err
		}

		if cmd != "login" {
			if err := c.Login(); err != nil {
				return err
			}
		}
	}

	switch cmd {
	case "login":
		data["username"] = c.Client.Username
		data["password"] = c.Client.Password
	case "user":
	case "captcha":
	case "upload":
	default:
		return fmt.Errorf("Unknown command")
	}

	data["cmd"] = cmd
	data["version"] = API_VERSION

	msg, err := json.Marshal(data)

	_, err = c.Connection.Write(append(msg, []byte("\n")...))
	if err != nil {
		return err
	}

	msg, err = bufio.NewReader(c.Connection).ReadBytes('\n')
	if err != nil {
		return err
	}

	err = json.Unmarshal(msg, target)
	if err != nil {
		return err
	}

	return nil
}

func (c *SocketClient) Login() error {
	var resp UserResp

	err := c.Call("login", nil, &resp)
	if err != nil {
		return err
	}

	if resp.Status != 0 {
		return fmt.Errorf("Unable to login")
	}

	return nil
}

func (c *SocketClient) Balance() (float64, error) {
	var resp UserResp

	err := c.Call("user", nil, &resp)
	if err != nil {
		return 0, err
	}

	return resp.Balance, nil
}

func (c *SocketClient) Recaptcha(imgData []byte) ([]RCoord, error) {
	var err error
	var width, height int

	if len(imgData) > MAXIMUM_IMG_SIZE {
		imgData, width, height, err = ResizeImage(imgData)
		if err != nil {
			return nil, err
		}
	}

	base64Img := base64.StdEncoding.EncodeToString(imgData)

	data := map[string]interface{}{
		"captcha": base64Img,
		"type":    2,
	}

	var resp struct {
		Status    int
		CaptchaID int  `json:"captcha"`
		IsCorrect bool `json:"is_correct"`
		Text      string
	}

	err = c.Call("upload", data, &resp)
	if err != nil {
		return nil, err
	}

	if !resp.IsCorrect {
		return nil, fmt.Errorf("Incorrect DBC Captcha")
	}

	for resp.Text == "" {
		time.Sleep(c.Config.PollInterval() * time.Second)

		c.Call("captcha", map[string]interface{}{"captcha": resp.CaptchaID}, &resp)
	}

	var coords [][]int
	err = json.Unmarshal([]byte(resp.Text), &coords)
	if err != nil {
		return nil, fmt.Errorf("Unexpected value in field \"text\" of upload response")
	}

	rcoords := make([]RCoord, len(coords))
	for i, _ := range rcoords {
		x := coords[i][0]
		y := coords[i][1]
		rcoord := RCoord{
			X: float64(x) / float64(width),
			Y: float64(y) / float64(height),
		}

		rcoords[i] = rcoord
	}

	return rcoords, nil
}

func ResizeImage(img []byte) ([]byte, int, int, error) {
	orgImg, _, err := image.Decode(bytes.NewReader(img))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("Invalid image")
	}

	orgWidth := orgImg.Bounds().Max.X
	orgHeight := orgImg.Bounds().Max.Y

	imgBuffer := new(bytes.Buffer)

	var i int
	var width float64
	var height float64
	// 143000 => 143kb: DBC has a maximum of 192kb per captcha (after base64).
	// Base64 increases size by 33% in worst case, and thereby the maximum is
	// set to 143kb
	// Developer hint: For some reason DBC won't accept JPEGs encoded by Go.
	for imgBuffer.Len() > MAXIMUM_IMG_SIZE || imgBuffer.Len() == 0 {
		imgBuffer.Reset()

		modifer := 1.0 - (float64(i) / 10.0)
		width = float64(orgWidth) * modifer
		height = float64(orgHeight) * modifer

		img := resize.Resize(uint(width), 0, orgImg, resize.Lanczos3)
		png.Encode(imgBuffer, img)

		i++
	}

	return imgBuffer.Bytes(), int(width), int(height), nil
}
