package cf

import (
	"time"
)

// SavedCookie is a simple serializable cookie struct returned by SpawnChromeAndCollectCFCookies.
type SavedCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	HttpOnly bool      `json:"http_only"`
	Secure   bool      `json:"secure"`
}

