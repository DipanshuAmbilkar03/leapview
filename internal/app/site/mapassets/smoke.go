// Package mapassets serves the public site's immutable basemap package and proves
// that a running site delivers every file its pages reference.
package mapassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/flidai/leapview/internal/dashboard/visualization/mapasset"
)

const archiveMagic = "PMTiles"

// Verify requests every file in the compiled map package from the site at
// baseURL. Each file must answer 200 with its contract content type; the style
// must match its pinned digest and the tile archive must serve byte ranges.
// Every failure is reported, so one run shows the complete damage.
func Verify(ctx context.Context, client *http.Client, baseURL string) error {
	origin, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host == "" || origin.User != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.Fragment != "" {
		return fmt.Errorf("site base URL must be an HTTP(S) origin: %q", baseURL)
	}
	origin.Path = ""
	if client == nil {
		client = http.DefaultClient
	}
	var failures []error
	for _, file := range mapasset.ExpectedFiles() {
		target := *origin
		target.Path = "/map-assets/" + file.Path
		if err := verifyFile(ctx, client, &target, file); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", file.Path, err))
		}
	}
	return errors.Join(failures...)
}

func verifyFile(ctx context.Context, client *http.Client, target *url.URL, file mapasset.File) error {
	head, err := do(ctx, client, http.MethodHead, target, "")
	if err != nil {
		return err
	}
	head.Body.Close()
	if err := expect(head, http.StatusOK, mapasset.ContentType(file.Path)); err != nil {
		return err
	}
	switch {
	case file.Digest == mapasset.StyleSHA256 && strings.HasSuffix(file.Path, "/style.json"):
		return verifyStyle(ctx, client, target)
	case file.Digest == mapasset.ArchiveSHA256:
		return verifyArchiveRange(ctx, client, target)
	}
	return nil
}

func verifyStyle(ctx context.Context, client *http.Client, target *url.URL) error {
	response, err := do(ctx, client, http.MethodGet, target, "")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expect(response, http.StatusOK, "application/json"); err != nil {
		return err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, response.Body); err != nil {
		return fmt.Errorf("read style: %w", err)
	}
	if digest := hex.EncodeToString(hash.Sum(nil)); digest != mapasset.StyleSHA256 {
		return fmt.Errorf("style digest %s, want %s", digest, mapasset.StyleSHA256)
	}
	return nil
}

func verifyArchiveRange(ctx context.Context, client *http.Client, target *url.URL) error {
	response, err := do(ctx, client, http.MethodGet, target, "bytes=0-"+fmt.Sprint(len(archiveMagic)-1))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := expect(response, http.StatusPartialContent, "application/vnd.pmtiles"); err != nil {
		return err
	}
	header, err := io.ReadAll(io.LimitReader(response.Body, int64(len(archiveMagic))))
	if err != nil {
		return fmt.Errorf("read archive range: %w", err)
	}
	if string(header) != archiveMagic {
		return fmt.Errorf("archive range starts with %q, want %q", header, archiveMagic)
	}
	return nil
}

func do(ctx context.Context, client *http.Client, method string, target *url.URL, byteRange string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	return response, nil
}

func expect(response *http.Response, status int, contentType string) error {
	if response.StatusCode != status {
		return fmt.Errorf("%s returned HTTP %d, want HTTP %d", response.Request.Method, response.StatusCode, status)
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != contentType {
		return fmt.Errorf("%s Content-Type %q, want %q", response.Request.Method, response.Header.Get("Content-Type"), contentType)
	}
	return nil
}
