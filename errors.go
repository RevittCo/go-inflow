package inflow

import (
	"errors"
	"fmt"
)

var ErrRateLimited = errors.New("rate limited (429)")

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("inflow api error (status %d): %s", e.StatusCode, e.Body)
}

func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

func IsAPIError(err error) *APIError {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return nil
}
