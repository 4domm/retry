package retry

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

var retryable = map[int]bool{
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

const (
	failureThreshold = 3
	openTimeout      = 10 * time.Second
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            circuitState
	failures         int
	openUntil        time.Time
	halfOpenInFlight bool
}

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{state: stateClosed}
}

func (cb *CircuitBreaker) allowRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case stateClosed:
		return nil
	case stateOpen:
		if now.After(cb.openUntil) {
			cb.state = stateHalfOpen
			cb.halfOpenInFlight = false
			log.Printf("circuit: Open to Half-Open")
		} else {
			return fmt.Errorf("circuit open until %s", cb.openUntil.Format(time.RFC3339))
		}
	case stateHalfOpen:
		if cb.halfOpenInFlight {
			return fmt.Errorf("circuit half-open: test request already in flight")
		}
		cb.halfOpenInFlight = true
		return nil
	}

	if cb.state == stateHalfOpen {
		cb.halfOpenInFlight = true
	}

	return nil
}

func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateHalfOpen:
		cb.state = stateClosed
		cb.failures = 0
		cb.halfOpenInFlight = false
		log.Printf("circuit: Half-Open to Closed")
	case stateClosed:
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateHalfOpen:
		cb.state = stateOpen
		cb.openUntil = time.Now().Add(openTimeout)
		cb.failures = 0
		cb.halfOpenInFlight = false
		log.Printf("circuit: Half-Open to Open")
	case stateClosed:
		cb.failures++
		if cb.failures >= failureThreshold {
			cb.state = stateOpen
			cb.openUntil = time.Now().Add(openTimeout)
			cb.failures = 0
			log.Printf("circuit: Closed to Open")
		}
	}
}

var defaultCircuitBreaker = NewCircuitBreaker()

func GetDataWithCircuitBreaker(url string) (string, error) {
	if err := defaultCircuitBreaker.allowRequest(); err != nil {
		return "", err
	}

	client := &http.Client{}
	body, status, err := doRequest(client, url)
	if err != nil {
		defaultCircuitBreaker.onFailure()
		return "", err
	}

	if status == http.StatusOK {
		defaultCircuitBreaker.onSuccess()
		return body, nil
	}

	if retryable[status] {
		defaultCircuitBreaker.onFailure()
		return "", fmt.Errorf("retryable status code %d", status)
	}

	defaultCircuitBreaker.onSuccess()
	return "", fmt.Errorf("unexpected status code %d", status)
}

func doRequest(client *http.Client, url string) (string, int, error) {
	resp, err := client.Get(url)
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
}
