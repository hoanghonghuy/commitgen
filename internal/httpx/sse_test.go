package httpx

import (
	"strings"
	"testing"
)

func TestReadSSEJSON_EventBoundary(t *testing.T) {
	// A complete JSON object can still have another data field containing
	// whitespace. Deliver the entire event, including that trailing field.
	body := "event: message\r\ndata:{\"text\":\"complete\"}\r\ndata:  \r\n\r\n"
	var events []string
	err := ReadSSEJSON(strings.NewReader(body), func(data []byte) (bool, error) {
		events = append(events, string(data))
		return false, nil
	})
	if err != nil || len(events) != 1 || events[0] != "{\"text\":\"complete\"}\n " {
		t.Errorf("SSE event was delivered before its boundary: events=%q err=%v", events, err)
	}
}
