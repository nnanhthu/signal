package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"math"
	"net/http"
	"sync"
	"time"
)

type Transport struct {
	Url    string
	token  string
	client http.Client

	requestID uint64
	reqMutex  sync.Mutex
}

func NewTransport(url, token string, timeout time.Duration) (*Transport, error) {
	return &Transport{
		client: http.Client{
			Timeout: timeout,
		},
		Url:   url,
		token: token,
	}, nil
}

func (caller *Transport) Call(method string, args interface{}) (interface{}, int, error) {
	caller.reqMutex.Lock()
	defer caller.reqMutex.Unlock()

	// increase request id
	if caller.requestID == math.MaxUint64 {
		caller.requestID = 0
	}
	caller.requestID++

	jsonValue, err := json.Marshal(args)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	var request *http.Request
	if method == "GET" {
		request, err = http.NewRequest(method, caller.Url, nil)
	} else {
		request, err = http.NewRequest(method, caller.Url, bytes.NewBuffer(jsonValue))
	}
	//request, err := netClient.Post(dest, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", caller.token)

	resp, err := caller.client.Do(request)

	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	respBody, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, http.StatusServiceUnavailable, errors.Wrap(err, "failed to read body")
	}
	if respBody != nil && len(respBody) > 0 {
		var reply interface{}
		if err = json.Unmarshal(respBody, &reply); err != nil {
			return nil, http.StatusServiceUnavailable, errors.Wrapf(err, "failed to unmarshal response: %+v", string(respBody))
		}
		return reply, resp.StatusCode, nil
	}
	return nil, resp.StatusCode, nil
}

func (caller *Transport) SetCallback(api string, method string, notice func(args json.RawMessage)) error {
	panic("not supported")
}

func (caller *Transport) Close() error {
	return nil
}

func check(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.Errorf("Error. URL: %s STATUS: %s\n", url, resp.StatusCode)
	}
	return nil
}
