package stickybind

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Lease is a tiny single-owner marker file. CPA can load a new plugin
// build while an older one keeps running inside the same process (shared
// objects cannot be unloaded, and the old instance's shutdown hook is not
// guaranteed to run on hot upgrade). Each instance claims the lease when
// its sync loop starts; newest claim wins. An instance that finds the lease
// owned by someone else stops syncing, so two binders with different
// configurations can never fight over the same credential.
type Lease struct {
	Path string
	ID   string
}

// NewInstanceID returns a random identifier for one plugin instance.
func NewInstanceID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("pid%d", os.Getpid())
	}
	return hex.EncodeToString(buf[:])
}

// Claim records this instance as the current owner (atomic replace).
func (l Lease) Claim() error {
	if strings.TrimSpace(l.Path) == "" || l.ID == "" {
		return fmt.Errorf("lease path or id is empty")
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%s.tmp", l.Path, l.ID)
	if err := os.WriteFile(tmp, []byte(l.ID+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.Path)
}

// Owner returns the instance id currently holding the lease ("" if none).
func (l Lease) Owner() (string, error) {
	raw, err := os.ReadFile(l.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// Held reports whether this instance still owns the lease. Read errors and a
// missing file fail open (keep syncing) so a broken lease dir never disables
// the plugin; only a positive "someone else owns it" stops the caller.
func (l Lease) Held() (held bool, owner string) {
	owner, err := l.Owner()
	if err != nil || owner == "" {
		return true, owner
	}
	return owner == l.ID, owner
}
