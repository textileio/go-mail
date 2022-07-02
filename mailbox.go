package mail

import "github.com/textileio/go-threads/core/thread"

type Config struct {
	// Path is the path in which the new mailbox should be created (required).
	Path string
	// Identity is the thread.Identity of the mailbox owner (required).
	// It's value may be inflated from a --identity flag or {EnvPrefix}_IDENTITY env variable.
	Identity thread.Identity
	// APIKey is hub API key (required).
	// It's value may be inflated from a --api-key flag or {EnvPrefix}_API_KEY env variable.
	APIKey string
	// APISecret is hub API key secret (optional).
	// It's value may be inflated from a --api-secret flag or {EnvPrefix}_API_SECRET env variable.
	APISecret string
}
