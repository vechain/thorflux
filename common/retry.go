package common

import (
	"fmt"
	"time"
)

func Retry(fn func() error, retryPeriod, maxWaitTime time.Duration) error {
	startTime := time.Now()
	for {
		err := fn()
		if err == nil {
			// If the function succeeds, return nil error
			return nil
		}

		if time.Since(startTime) > maxWaitTime {
			// If maxWaitTime has been exceeded, return the last error
			return fmt.Errorf("retry timeout, latest err: %w", err)
		}

		// Wait for the retryPeriod before retrying
		time.Sleep(retryPeriod)
	}
}

func RetryIncreasing(fn func() error, initialDelay time.Duration, maxDelay time.Duration, maxWaitTime time.Duration) error {
	startTime := time.Now()
	delay := initialDelay

	for {
		err := fn()
		if err == nil {
			// If the function succeeds, return nil error
			return nil
		}

		if time.Since(startTime) > maxWaitTime {
			// If maxWaitTime has been exceeded, return the last error
			return fmt.Errorf("retry timeout, latest err: %w", err)
		}

		// Wait for the current delay before retrying
		time.Sleep(delay)

		// Increase the delay for the next retry, capped at maxDelay
		delay *= 10
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}
