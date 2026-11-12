package httpx

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const maxSSEEventSize = 1024 * 1024

// NewSSEScanner reads LF, CRLF and CR lines and strips one leading UTF-8 BOM.
func NewSSEScanner(body io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSEEventSize)
	firstLine, skipLF := true, false
	line := func(data []byte) []byte {
		if firstLine {
			firstLine = false
			return bytes.TrimPrefix(data, []byte("\ufeff"))
		}
		return data
	}
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		offset := 0
		if skipLF && len(data) > 0 {
			skipLF = false
			if data[0] == '\n' {
				data = data[1:]
				offset = 1
			}
		}
		for i, b := range data {
			switch b {
			case '\r':
				// Consume CR immediately; an optional LF may arrive in the next read.
				skipLF = true
				return offset + i + 1, line(data[:i]), nil
			case '\n':
				return offset + i + 1, line(data[:i]), nil
			}
		}
		if atEOF && len(data) > 0 {
			return offset + len(data), line(data), nil
		}
		return offset, nil, nil
	})
	return scanner
}

// ReadSSEJSON joins data fields and delivers JSON only at the event boundary.
// The callback returns true to stop reading after a terminal event.
func ReadSSEJSON(body io.Reader, visit func([]byte) (bool, error)) error {
	scanner := NewSSEScanner(body)
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
