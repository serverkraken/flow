package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// CheckMeta fetches /api/v1/meta (public, no Bearer) and updates the status
// with the server version. If the client version is below min_client_version,
// the status is set to StateOutdated (sticky until restart).
func (c *Client) CheckMeta(ctx context.Context) error {
	if c.base == "" {
		return ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/meta", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Flow-Client-Version", c.version)
	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("httpapi: /meta status %d", resp.StatusCode)
	}
	var m metaDTO
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return fmt.Errorf("httpapi: decode /meta: %w", err)
	}
	c.status.set(func(sn *StatusSnapshot) {
		sn.ServerVersion = m.ServerVersion
		if c.version != "dev" && m.MinClientVersion != "" && versionLess(c.version, m.MinClientVersion) {
			sn.State = StateOutdated
		}
	})
	return nil
}

// versionLess reports whether dotted-numeric version a < b.
// "dev" is never less than anything.
func versionLess(a, b string) bool {
	if a == "dev" {
		return false
	}
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	n := len(aParts)
	if len(bParts) > n {
		n = len(bParts)
	}
	for i := 0; i < n; i++ {
		aNum := partInt(aParts, i)
		bNum := partInt(bParts, i)
		if aNum < bNum {
			return true
		}
		if aNum > bNum {
			return false
		}
	}
	return false
}

func partInt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}
