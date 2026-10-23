package openai

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadProviderBodyEnforcesLimit(t *testing.T) {
	got, err := readProviderBody(strings.NewReader("12345"), 5)
	if err != nil || string(got) != "12345" {
		t.Fatalf("at limit: %q, %v", got, err)
	}
	if _, err := readProviderBody(strings.NewReader("123456"), 5); !errors.Is(err, errProviderResponseTooLarge) {
		t.Fatalf("over limit error = %v", err)
	}
}

func TestReadProviderErrorBodyIsTruncated(t *testing.T) {
	got := readProviderErrorBody(strings.NewReader(strings.Repeat("x", maxProviderErrorBodyBytes*4)))
	if len(got) != maxProviderErrorBodyBytes {
		t.Fatalf("error body length = %d, want %d", len(got), maxProviderErrorBodyBytes)
	}
}

func TestReadBoundedLine(t *testing.T) {
	// A tiny bufio buffer forces the multi-chunk path.
	reader := bufio.NewReaderSize(strings.NewReader("short\n"+strings.Repeat("a", 100)+"\nlast"), 16)
	if line, err := readBoundedLine(reader, 200); line != "short\n" || err != nil {
		t.Fatalf("first line = %q, %v", line, err)
	}
	if line, err := readBoundedLine(reader, 200); line != strings.Repeat("a", 100)+"\n" || err != nil {
		t.Fatalf("long line = %q, %v", line, err)
	}
	if line, err := readBoundedLine(reader, 200); line != "last" || !errors.Is(err, io.EOF) {
		t.Fatalf("final line = %q, %v", line, err)
	}

	reader = bufio.NewReaderSize(strings.NewReader(strings.Repeat("b", 100)+"\n"), 16)
	if _, err := readBoundedLine(reader, 50); !errors.Is(err, errProviderResponseTooLarge) {
		t.Fatalf("oversized line error = %v", err)
	}
}

func TestReadSSEDataRejectsOversizedLine(t *testing.T) {
	input := "data: first\n\ndata: " + strings.Repeat("z", 1000) + "\n\n"
	reader := bufio.NewReader(strings.NewReader(input))
	if data, err := readSSEDataLimit(reader, 64); data != "first" || err != nil {
		t.Fatalf("first event = %q, %v", data, err)
	}
	if _, err := readSSEDataLimit(reader, 64); !errors.Is(err, errProviderResponseTooLarge) {
		t.Fatalf("oversized event error = %v", err)
	}
	// An oversized line after a partial event must not flush the partial data.
	reader = bufio.NewReader(strings.NewReader("data: partial\n" + strings.Repeat("z", 1000) + "\n"))
	if data, err := readSSEDataLimit(reader, 64); !errors.Is(err, errProviderResponseTooLarge) || data != "" {
		t.Fatalf("partial event = %q, %v", data, err)
	}
}
