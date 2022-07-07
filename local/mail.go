package local

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	mailClient "github.com/textileio/go-mail/api/client"
	"github.com/textileio/go-mail/cmd"
	threadClient "github.com/textileio/go-threads/api/client"
	"github.com/textileio/go-threads/core/thread"
)

var (
	// ErrNotAMailbox indicates the given path is not within a mailbox.
	ErrNotAMailbox = errors.New("not a mailbox (or any of the parent directories): .textile")
	// ErrMailboxExists is used during initialization to indicate the path already contains a mailbox.
	ErrMailboxExists = errors.New("mailbox is already initialized")
	// ErrIdentityRequired indicates the operation requires a thread Identity but none was given.
	ErrIdentityRequired = errors.New("thread Identity is required")
	// ErrAPIKeyRequired indicates the operation requires am API keybut none was given.
	ErrAPIKeyRequired = errors.New("api key is required")

	flags = map[string]cmd.Flag{
		"key":      {Key: "key", DefValue: ""},
		"thread":   {Key: "thread", DefValue: ""},
		"identity": {Key: "identity", DefValue: ""},
	}
)

// DefaultConfConfig returns the default ConfConfig.
func DefaultConfConfig() cmd.ConfConfig {
	return cmd.ConfConfig{
		Dir:       ".textile",
		Name:      "config",
		Type:      "yaml",
		EnvPrefix: "MAIL",
	}
}

// Mail is used to create new mailboxes based on the provided client and config.
type Mail struct {
	conf cmd.ConfConfig
	mc   *mailClient.Client
	tc   *threadClient.Client
}

// NewMail creates Mail from client and config.
func NewMail(mailClient *mailClient.Client, threadClient *threadClient.Client, config cmd.ConfConfig) *Mail {
	return &Mail{mc: mailClient, tc: threadClient, conf: config}
}

// client returns the underlying client object.
func (m *Mail) MailClient() *mailClient.Client {
	return m.mc
}

func (m *Mail) ThreadClient() *threadClient.Client {
	return m.tc
}

// Config contains details for a new local mailbox.
type Config struct {
	// Path is the path in which the new mailbox should be created (required).
	Path string
	// Identity is an identity to use with the target thread.
	// It's value may be inflated from an --identity flag or {EnvPrefix}_IDENTITY env variable.
	// @todo: Handle more identities
	// @todo: Pull this from a global config of identites, i.e., ~/.threads:
	//   identities:
	//     default: clyde
	//     clyde: <priv_key_base_64>
	//     eddy: <priv_key_base_64>
	Identity thread.Identity

	// Thread is the thread ID of the target thread (required).
	// Its value may be inflated from a --thread flag or {EnvPrefix}_THREAD env variable.
	Thread thread.ID
}

// NewConfigFromCmd returns a config by inflating values from the given cobra command and path.
func (m *Mail) NewConfigFromCmd(c *cobra.Command, pth string) (conf Config, err error) {
	conf.Path = pth
	id := cmd.GetFlagOrEnvValue(c, "identity", m.conf.EnvPrefix)
	if id == "" {
		return conf, ErrIdentityRequired
	}
	idb, err := base64.StdEncoding.DecodeString(id)
	if err != nil {
		return
	}
	conf.Identity = &thread.Libp2pIdentity{}
	if err = conf.Identity.UnmarshalBinary(idb); err != nil {
		return
	}

	return conf, nil
}

// NewMailbox initializes a new mailbox from the config.
func (m *Mail) NewMailbox(ctx context.Context, conf Config) (box *Mailbox, err error) {
	// Ensure we're not going to overwrite an existing local config
	cwd, err := filepath.Abs(conf.Path)
	if err != nil {
		return
	}
	mc, found, err := m.conf.NewConfig(cwd, flags, true)
	if err != nil {
		return
	}
	if found {
		return nil, ErrMailboxExists
	}

	// Check config values
	if conf.Identity == nil {
		return nil, ErrIdentityRequired
	}
	idb, err := conf.Identity.MarshalBinary()
	if err != nil {
		return
	}
	mc.Viper.Set("identity", base64.StdEncoding.EncodeToString(idb))

	box = &Mailbox{
		cwd:  cwd,
		conf: mc,
		mc:   m.mc,
		tc:   m.tc,
		id:   conf.Identity,
	}

	// Write the local config to disk
	dir := filepath.Join(cwd, box.conf.Dir)
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		return
	}
	config := filepath.Join(dir, box.conf.Name+".yml")
	if err = box.conf.Viper.WriteConfigAs(config); err != nil {
		return
	}
	cfile, err := filepath.Abs(config)
	if err != nil {
		return
	}
	box.conf.Viper.SetConfigFile(cfile)

	return box, nil
}

// GetLocalMailbox loads and returns the mailbox at path if it exists.
func (m *Mail) GetLocalMailbox(_ context.Context, pth string) (*Mailbox, error) {
	cwd, err := filepath.Abs(pth)
	if err != nil {
		return nil, err
	}
	conf, found, err := m.conf.NewConfig(cwd, flags, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotAMailbox
	}
	cmd.ExpandConfigVars(conf.Viper, conf.Flags)
	box := &Mailbox{
		cwd:  cwd,
		conf: conf,
		mc:   m.mc,
		tc:   m.tc,
	}
	if err = box.loadIdentity(); err != nil {
		return nil, err
	}
	return box, nil
}
