package retry

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

var retryable = map[int]bool{
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

func GetData(url string) (string, error) {
	const maxAttempts = 3
	const baseDelay = 1 * time.Second
	const maxDelay = 2 * time.Second
	const multiplier = 2.0

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if delay := backoffDelay(attempt, baseDelay, maxDelay, multiplier); delay > 0 {
			time.Sleep(delay)
		}

		body, status, err := func() (string, int, error) {
			resp, err := http.Get(url)
			if err != nil {
				return "", 0, err
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				data, readErr := io.ReadAll(resp.Body)
				if readErr != nil {
					return "", 0, readErr
				}
				return string(data), resp.StatusCode, nil
			}

			return "", resp.StatusCode, nil
		}()
		if err != nil {
			return "", err
		}

		if status == http.StatusOK {
			return body, nil
		}

		if !retryable[status] {
			return "", fmt.Errorf("unexpected status code %d", status)
		}

		if attempt == maxAttempts {
			return "", fmt.Errorf("retryable status code %d after %d attempts", status, maxAttempts)
		}
	}

	return "", fmt.Errorf("exhausted retries")
}

func backoffDelay(attempt int, baseDelay, maxDelay time.Duration, multiplier float64) time.Duration {
	if attempt <= 1 {
		return 0
	}
	delay := time.Duration(float64(baseDelay) * math.Pow(multiplier, float64(attempt-2)))
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}
