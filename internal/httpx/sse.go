package httpx

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxSSEEventSize = 1024 * 1024

// ReadSSEJSON joins data fields and delivers JSON only at the event boundary.
// The callback returns true to stop reading after a terminal event.
func ReadSSEJSON(body io.Reader, visit func([]byte) (bool, error)) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSEEventSize)
	var pending strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if pending.Len() == 0 {
				continue
			}
			data := []byte(pending.String())
			if !json.Valid(data) {
				return fmt.Errorf("invalid JSON in SSE event")
			}
			stop, err := visit(data)
			if err != nil {
				return err
			}
			pending.Reset()
			if stop {
				return nil
			}
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimPrefix(data, " ")
		if pending.Len()+len(data)+1 > maxSSEEventSize {
			return fmt.Errorf("SSE event exceeds %d bytes", maxSSEEventSize)
		}
		if pending.Len() > 0 {
			pending.WriteByte('\n')
		}
		pending.WriteString(data)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if pending.Len() > 0 {
		return fmt.Errorf("incomplete SSE event: %w", io.ErrUnexpectedEOF)
	}
	return nil
}
