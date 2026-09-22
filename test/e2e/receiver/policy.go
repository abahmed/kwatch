package main

import (
	"fmt"
	"net/http"
	"time"
)

type Policy struct {
	Mode  string `json:"mode"`
	Delay string `json:"delay,omitempty"`
}

func (p Policy) response() (int, time.Duration, error) {
	switch p.Mode {
	case "", "success":
		return http.StatusOK, 0, nil
	case "http-500":
		return http.StatusInternalServerError, 0, nil
	case "http-429":
		return http.StatusTooManyRequests, 0, nil
	case "delay":
		delay, err := time.ParseDuration(p.Delay)
		return http.StatusOK, delay, err
	case "connection-reset":
		return 0, 0, nil
	default:
		return 0, 0, fmt.Errorf("unknown receiver mode %q", p.Mode)
	}
}
