package handler

import (
	"time"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/correlation"
)

var testAlertMgr = &alert.AlertManager{}

func testCorrelator() *correlation.Engine {
	return correlation.NewEngine(correlation.Config{
		Window: 10 * time.Minute,
	})
}
