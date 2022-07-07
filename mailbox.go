package mail

import "github.com/textileio/go-threads/core/thread"

type Config struct {
	// Path is the path in which the new mailbox should be created (required).
	Path string
	// Identity is the thread.Identity of the mailbox owner (required).
	// It's value may be inflated from a --identity flag or {EnvPrefix}_IDENTITY env variable.
	Identity thread.Identity
}
