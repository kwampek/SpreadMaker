package util

import (
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type Client struct {
	http.Client
	limiter *rate.Limiter
}

func NewClient(rps float64, burst int) *Client {
	limiter := rate.NewLimiter(rate.Limit(rps), burst)

	// Need to add proxy
	return &Client{
		Client:  http.Client{},
		limiter: limiter,
	}
}

func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	return c.Do(req)
}

func (c *Client) SimpleDo(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	return c.Client.Do(req)
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.SimpleDo(req)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		time.Sleep(1 * time.Second)
		return c.SimpleDo(req)
	}

	return resp, nil
}
