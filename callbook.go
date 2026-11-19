package main

import (
	"net/http"
	"time"

	"jstlogger/internal/callbook"
	"jstlogger/internal/config"
)

// callbookTimeout bounds one request to a callbook service. A login holds a
// lock that other lookups wait on, so a stalled service must not hang them.
const callbookTimeout = 10 * time.Second

// newCallbook returns the lookup configured by cfg: its services, tried in
// order, behind a cache keeping found calls for cfg.CacheTTL. It returns
// nil when no service is configured. Load and Validate have already
// normalized the service names and checked their credentials.
func newCallbook(cfg config.Callbook) callbook.Looker {
	if len(cfg.Services) == 0 {
		return nil
	}
	client := &http.Client{Timeout: callbookTimeout}
	chain := make(callbook.Chain, 0, len(cfg.Services))
	for _, service := range cfg.Services {
		switch service {
		case config.CallbookQRZ:
			chain = append(chain, &callbook.QRZ{
				Username: cfg.QRZ.Username,
				Password: cfg.QRZ.Password,
				FullName: cfg.QRZ.FullName,
				Client:   client,
			})
		case config.CallbookHamQTH:
			chain = append(chain, &callbook.HamQTH{
				Username: cfg.HamQTH.Username,
				Password: cfg.HamQTH.Password,
				Client:   client,
			})
		}
	}
	return &callbook.Cache{Next: chain, TTL: cfg.CacheTTL}
}
