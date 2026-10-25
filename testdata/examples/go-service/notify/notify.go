// Package notify tells the warehouse about new orders. It reads the
// clock, the environment and the network directly, which is what makes it
// hard to test.
package notify

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"example.com/orders/order"
)

// Send posts an order to the warehouse webhook, trying up to three times.
func Send(o order.Order) error {
	url := os.Getenv("WAREHOUSE_URL")
	if url == "" {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"order": o, "sentAt": time.Now().UTC()})
	attempts := 0
retry:
	attempts++
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		if attempts < 3 {
			time.Sleep(time.Duration(attempts) * 100 * time.Millisecond)
			goto retry
		}
		return err
	}
	return resp.Body.Close()
}
