package downloader

import (
	"encoding/json"
	"os"
	"sync"
)

var jsonMu sync.Mutex

func writeJSONEvent(event string, fields map[string]any) {
	if fields == nil {
		fields = make(map[string]any)
	}
	fields["event"] = event
	jsonMu.Lock()
	json.NewEncoder(os.Stdout).Encode(fields)
	jsonMu.Unlock()
}
