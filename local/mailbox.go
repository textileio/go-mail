package local

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/textileio/go-mail/api/client"
	mailClient "github.com/textileio/go-mail/api/client"
	"github.com/textileio/go-mail/cmd"
	"github.com/textileio/go-mail/collection"
	threadClient "github.com/textileio/go-threads/api/client"
	"github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/thread"
)

// Mailbox is a local-first messaging library built on ThreadDB and IPFS.
type Mailbox struct {
	cwd  string
	conf *cmd.Config
	id   thread.Identity

	mc *mailClient.Client
	tc *threadClient.Client
}

// Identity returns the mailbox's identity.
func (m *Mailbox) Identity() thread.Identity {
	return m.id
}

// SendMessage sends the message body to a recipient.
func (m *Mailbox) SendMessage(ctx context.Context, to thread.PubKey, body []byte) (msg client.Message, err error) {
	ctx, err = m.context(ctx)
	if err != nil {
		return
	}
	return m.mc.SendMessage(ctx, m.id, to, body)
}

// ListInboxMessages lists messages from the inbox.
// Use options to paginate with seek and limit,
// and filter by read status.
func (m *Mailbox) ListInboxMessages(ctx context.Context, opts ...client.ListOption) ([]client.Message, error) {
	ctx, err := m.context(ctx)
	if err != nil {
		return nil, err
	}
	return m.mc.ListInboxMessages(ctx, opts...)
}

// ListSentboxMessages lists messages from the sentbox.
// Use options to paginate with seek and limit.
func (m *Mailbox) ListSentboxMessages(ctx context.Context, opts ...client.ListOption) ([]client.Message, error) {
	ctx, err := m.context(ctx)
	if err != nil {
		return nil, err
	}
	return m.mc.ListSentboxMessages(ctx, opts...)
}

const reconnectInterval = time.Second * 5

// MailboxEvent describes an event that occurred in a mailbox.
type MailboxEvent struct {
	// Type of event.
	Type MailboxEventType
	// Message identifier.
	MessageID db.InstanceID
	// Message will contain the full message unless this is a delete event.
	Message client.Message
}

// MailboxEventType is the type of mailbox event.
type MailboxEventType int

const (
	// NewMessage indicates the mailbox has a new message.
	NewMessage MailboxEventType = iota
	// MessageRead indicates a message was read in the mailbox.
	MessageRead
	// MessageDeleted indicates a message was deleted from the mailbox.
	MessageDeleted
)

// WatchInbox watches the inbox for new mailbox events.
// If offline is true, this will keep watching during network interruptions.
// Returns a channel of watch connectivity states.
// Cancel context to stop watching.
func (m *Mailbox) WatchInbox(ctx context.Context, mevents chan<- MailboxEvent, offline bool) (<-chan cmd.WatchState, error) {
	ctx, err := m.context(ctx)
	if err != nil {
		return nil, err
	}
	box := thread.NewPubKeyIDV1(m.Identity().GetPublic())
	if !offline {
		return m.listenWhileConnected(ctx, box, collection.InboxCollectionName, mevents)
	}
	return cmd.Watch(ctx, func(ctx context.Context) (<-chan cmd.WatchState, error) {
		return m.listenWhileConnected(ctx, box, collection.InboxCollectionName, mevents)
	}, reconnectInterval)
}

// WatchSentbox watches the sentbox for new mailbox events.
// If offline is true, this will keep watching during network interruptions.
// Returns a channel of watch connectivity states.
// Cancel context to stop watching.
func (m *Mailbox) WatchSentbox(ctx context.Context, mevents chan<- MailboxEvent, offline bool) (<-chan cmd.WatchState, error) {
	ctx, err := m.context(ctx)
	if err != nil {
		return nil, err
	}

	box := thread.NewPubKeyIDV1(m.Identity().GetPublic())
	if !offline {
		return m.listenWhileConnected(ctx, box, collection.SentboxCollectionName, mevents)
	}
	return cmd.Watch(ctx, func(ctx context.Context) (<-chan cmd.WatchState, error) {
		return m.listenWhileConnected(ctx, box, collection.SentboxCollectionName, mevents)
	}, reconnectInterval)
}

// listenWhileConnected will listen until context is canceled or an error occurs.
func (m *Mailbox) listenWhileConnected(ctx context.Context, boxID thread.ID, boxName string, mevents chan<- MailboxEvent) (<-chan cmd.WatchState, error) {
	state := make(chan cmd.WatchState)
	go func() {
		defer close(state)

		// Start listening for remote changes
		events, err := m.tc.Listen(ctx, boxID, []threadClient.ListenOption{{
			Type:       threadClient.ListenAll,
			Collection: boxName,
		}})
		if err != nil {
			state <- cmd.WatchState{Err: err, Aborted: !cmd.IsConnectionError(err)}
			return
		}
		errs := make(chan error)
		go func() {
			for e := range events {
				if e.Err != nil {
					errs <- e.Err // events will close on error
					continue
				}
				switch e.Action.Type {
				case threadClient.ActionCreate, threadClient.ActionSave:
					var msg client.Message
					if err := msg.UnmarshalInstance(e.Action.Instance); err != nil {
						errs <- err
						return
					}
					var t MailboxEventType
					if e.Action.Type == threadClient.ActionCreate {
						t = NewMessage
					} else {
						t = MessageRead
					}
					mevents <- MailboxEvent{
						Type:      t,
						MessageID: db.InstanceID(e.Action.InstanceID),
						Message:   msg,
					}
				case threadClient.ActionDelete:
					mevents <- MailboxEvent{
						Type:      MessageDeleted,
						MessageID: db.InstanceID(e.Action.InstanceID),
					}
				}
			}
		}()

		// If we made it here, we must be online
		state <- cmd.WatchState{State: cmd.Online}

		for {
			select {
			case err := <-errs:
				state <- cmd.WatchState{Err: err, Aborted: !cmd.IsConnectionError(err)}
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return state, nil
}

// ReadInboxMessage marks a message as read by ID.
func (m *Mailbox) ReadInboxMessage(ctx context.Context, id string) error {
	ctx, err := m.context(ctx)
	if err != nil {
		return err
	}
	return m.mc.ReadInboxMessage(ctx, id)
}

// DeleteInboxMessage deletes an inbox message by ID.
func (m *Mailbox) DeleteInboxMessage(ctx context.Context, id string) error {
	ctx, err := m.context(ctx)
	if err != nil {
		return err
	}
	return m.mc.DeleteInboxMessage(ctx, id)
}

// DeleteSentboxMessage deletes a sent message by ID.
func (m *Mailbox) DeleteSentboxMessage(ctx context.Context, id string) error {
	ctx, err := m.context(ctx)
	if err != nil {
		return err
	}
	return m.mc.DeleteSentboxMessage(ctx, id)
}

// Identity returns the mailbox's user identity.
func (m *Mailbox) loadIdentity() error {
	ids := m.conf.Viper.GetString("identity")
	if ids == "" {
		return fmt.Errorf("identity not found")
	}
	idb, err := base64.StdEncoding.DecodeString(ids)
	if err != nil {
		return fmt.Errorf("loading identity: %v", err)
	}
	m.id = &thread.Libp2pIdentity{}
	if err = m.id.UnmarshalBinary(idb); err != nil {
		return fmt.Errorf("unmarshalling identity: %v", err)
	}
	return nil
}

func (m *Mailbox) context(ctx context.Context) (context.Context, error) {
	return m.mc.NewTokenContext(ctx, m.id, time.Second)
}
