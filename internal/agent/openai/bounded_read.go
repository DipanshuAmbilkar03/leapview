package openai

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

const (
	// maxProviderBodyBytes bounds a complete non-streaming provider response.
	maxProviderBodyBytes = 32 << 20
	// maxProviderErrorBodyBytes bounds error bodies, which are only inspected
	// for a context-length marker and never surfaced to callers.
	maxProviderErrorBodyBytes = 64 << 10
	// maxSSELineBytes bounds one SSE line. A terminal Responses event carries
	// the whole response on a single line, so this matches the body bound.
	maxSSELineBytes = maxProviderBodyBytes
)

var errProviderResponseTooLarge = errors.New("provider response exceeds the size limit")

// readProviderBody reads at most limit bytes and fails when the body is larger.
func readProviderBody(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w (%d bytes)", errProviderResponseTooLarge, limit)
	}
	return data, nil
}

// readProviderErrorBody returns the leading bytes of an error body. A truncated
// body is acceptable here because callers only match a marker near its start.
func readProviderErrorBody(body io.Reader) []byte {
	data, _ := io.ReadAll(io.LimitReader(body, maxProviderErrorBodyBytes))
	return data
}

// readBoundedLine reads one line including its terminator without buffering
// more than limit bytes. The final line may lack a terminator and is returned
// with io.EOF.
func readBoundedLine(reader *bufio.Reader, limit int) (string, error) {
	var line []byte
	for {
		chunk, err := reader.ReadSlice('\n')
		if len(line)+len(chunk) > limit {
			return "", fmt.Errorf("%w: SSE line longer than %d bytes", errProviderResponseTooLarge, limit)
		}
		line = append(line, chunk...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return string(line), err
	}
}
