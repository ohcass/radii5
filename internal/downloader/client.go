package downloader

import (
	"net/http"
	"sync"
	"time"
)

var httpClient = NewOptimizedHTTPClient()

var pool256k = sync.Pool{New: func() any { b := make([]byte, 256*1024); return b }}

const maxRetries = 10

func NewOptimizedHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxConnsPerHost:     0,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
			DisableCompression:  false,
			WriteBufferSize:     256 * 1024,
		},
		Timeout: 30 * time.Minute,
	}
}
