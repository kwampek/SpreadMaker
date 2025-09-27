package util

import (
	"net/http"
	"time"
)

// SafeOpenPage выполняет HTTP-запрос с повторными попытками при получении статуса 429.
// maxRetries - максимальное количество повторных попыток
// initialDelay - начальная задержка перед повторной попыткой
func safeOpenPage(client *http.Client, req *http.Request, maxRetries int, initialDelay time.Duration) (*http.Response, error) {
	var resp *http.Response
	var err error

	for i := 0; i <= maxRetries; i++ {
		resp, err = client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}

		resp.Body.Close()

		if i == maxRetries {
			break
		}

		delay := initialDelay * time.Duration(1<<uint(i))
		time.Sleep(delay)
	}

	return resp, nil
}

func SafeOpenPage(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return SafeOpenReq(client, req)
}

func SafeOpenReq(client *http.Client, req *http.Request) (*http.Response, error) {
	return safeOpenPage(client, req, 5, time.Second)
}
