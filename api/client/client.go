package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	pb "github.com/textileio/go-mail/api/pb/mail"
	"github.com/textileio/go-mail/collection"

	"github.com/textileio/go-threads/core/did"
	core "github.com/textileio/go-threads/core/thread"
	"google.golang.org/grpc"
)

type Client struct {
	c      pb.APIServiceClient
	conn   *grpc.ClientConn
	target did.DID
}

// NewClient starts the client.
func NewClient(addr string, opts ...grpc.DialOption) (*Client, error) {
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		c:      pb.NewAPIServiceClient(conn),
		conn:   conn,
		target: "did:key:foo", // @todo: Get target from thread services
	}, nil
}

// NewTokenContext adds an identity token targeting the client's service.
func (c *Client) NewTokenContext(
	ctx context.Context,
	identity core.Identity,
	duration time.Duration,
) (context.Context, error) {
	token, err := identity.Token(c.target, duration)
	if err != nil {
		return nil, err
	}
	return did.NewTokenContext(ctx, token), nil
}

// Close closes the client's grpc connection and cancels any active requests.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Message is the client side representation of a mailbox message.
// Signature corresponds to the encrypted body.
// Use message.Open to get the plaintext body.
type Message struct {
	ID        string      `json:"_id"`
	From      core.PubKey `json:"from"`
	To        core.PubKey `json:"to"`
	Body      []byte      `json:"body"`
	Signature []byte      `json:"signature"`
	CreatedAt time.Time   `json:"created_at"`
	ReadAt    time.Time   `json:"read_at,omitempty"`
}

// Open decrypts the message body with identity.
func (m Message) Open(ctx context.Context, id core.Identity) ([]byte, error) {
	return id.Decrypt(ctx, m.Body)
}

// IsRead returns whether or not the message has been read.
func (m Message) IsRead() bool {
	return !m.ReadAt.IsZero()
}

// UnmarshalInstance unmarshals the message from its ThreadDB instance data.
// This will return an error if the message signature fails verification.
func (m Message) UnmarshalInstance(data []byte) error {
	// InboxMessage works for both inbox and sentbox messages (it contains a superset of SentboxMessage fields)
	var tm collection.InboxMessage
	if err := json.Unmarshal(data, &tm); err != nil {
		return err
	}
	body, err := base64.StdEncoding.DecodeString(tm.Body)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(tm.Signature)
	if err != nil {
		return err
	}
	from := &core.Libp2pPubKey{}
	if err := from.UnmarshalString(tm.From); err != nil {
		return fmt.Errorf("from public key is invalid")
	}
	ok, err := from.Verify(body, sig)
	if !ok || err != nil {
		return fmt.Errorf("bad message signature")
	}
	to := &core.Libp2pPubKey{}
	if err := to.UnmarshalString(tm.To); err != nil {
		return fmt.Errorf("to public key is invalid")
	}
	readAt := time.Time{}
	if tm.ReadAt > 0 {
		readAt = time.Unix(0, tm.ReadAt)
	}
	m.ID = tm.ID
	m.From = from
	m.To = to
	m.Body = body
	m.Signature = sig
	m.CreatedAt = time.Unix(0, tm.CreatedAt)
	m.ReadAt = readAt
	return nil
}

// SendMessage sends the message body to a recipient.
func (c *Client) SendMessage(ctx context.Context, from core.Identity, to core.PubKey, body []byte) (msg Message, err error) {
	fromBody, err := from.GetPublic().Encrypt(body)
	if err != nil {
		return msg, err
	}
	fromSig, err := from.Sign(ctx, fromBody)
	if err != nil {
		return msg, err
	}
	toBody, err := to.Encrypt(body)
	if err != nil {
		return msg, err
	}
	toSig, err := from.Sign(ctx, toBody)
	if err != nil {
		return msg, err
	}
	res, err := c.c.SendMessage(ctx, &pb.SendMessageRequest{
		To:            to.String(),
		ToBody:        toBody,
		ToSignature:   toSig,
		FromBody:      fromBody,
		FromSignature: fromSig,
	})
	if err != nil {
		return msg, err
	}
	return Message{
		ID:        res.Id,
		From:      from.GetPublic(),
		To:        to,
		Body:      fromBody,
		Signature: fromSig,
		CreatedAt: time.Unix(0, res.CreatedAt),
	}, nil
}

// ListInboxMessages lists messages from the inbox.
// Use options to paginate with seek and limit and filter by read status.
func (c *Client) ListInboxMessages(ctx context.Context, opts ...ListOption) ([]Message, error) {
	args := &listOptions{
		status: All,
	}
	for _, opt := range opts {
		opt(args)
	}
	var s pb.MailboxMessageStatus
	switch args.status {
	case All:
		s = pb.MailboxMessageStatus_ALL
	case Read:
		s = pb.MailboxMessageStatus_READ
	case Unread:
		s = pb.MailboxMessageStatus_UNREAD
	default:
		return nil, fmt.Errorf("unknown status: %v", args.status)
	}
	res, err := c.c.ListInboxMessages(ctx, &pb.ListInboxMessagesRequest{
		Seek:      args.seek,
		Limit:     int64(args.limit),
		Ascending: args.ascending,
		Status:    s,
	})
	if err != nil {
		return nil, err
	}
	return handleMessageList(res.Messages)
}

// ListSentboxMessages lists messages from the sentbox.
// Use options to paginate with seek and limit.
func (c *Client) ListSentboxMessages(ctx context.Context, opts ...ListOption) ([]Message, error) {
	args := &listOptions{
		status: All,
	}
	for _, opt := range opts {
		opt(args)
	}
	res, err := c.c.ListSentboxMessages(ctx, &pb.ListSentboxMessagesRequest{
		Seek:  args.seek,
		Limit: int64(args.limit),
	})
	if err != nil {
		return nil, err
	}
	return handleMessageList(res.Messages)
}

func handleMessageList(list []*pb.Message) ([]Message, error) {
	msgs := make([]Message, len(list))
	var err error
	for i, m := range list {
		msgs[i], err = messageFromPb(m)
		if err != nil {
			return nil, err
		}
	}
	return msgs, nil
}

func messageFromPb(m *pb.Message) (msg Message, err error) {
	from := &core.Libp2pPubKey{}
	if err := from.UnmarshalString(m.From); err != nil {
		return msg, fmt.Errorf("from public key is invalid")
	}
	ok, err := from.Verify(m.Body, m.Signature)
	if !ok || err != nil {
		return msg, fmt.Errorf("bad message signature")
	}
	to := &core.Libp2pPubKey{}
	if err := to.UnmarshalString(m.To); err != nil {
		return msg, fmt.Errorf("to public key is invalid")
	}
	readAt := time.Time{}
	if m.ReadAt > 0 {
		readAt = time.Unix(0, m.ReadAt)
	}
	return Message{
		ID:        m.Id,
		From:      from,
		To:        to,
		Body:      m.Body,
		Signature: m.Signature,
		CreatedAt: time.Unix(0, m.CreatedAt),
		ReadAt:    readAt,
	}, nil
}

// ReadInboxMessage marks a message as read by ID.
func (c *Client) ReadInboxMessage(ctx context.Context, id string) error {
	_, err := c.c.ReadInboxMessage(ctx, &pb.ReadInboxMessageRequest{
		Id: id,
	})
	return err
}

// DeleteInboxMessage deletes an inbox message by ID.
func (c *Client) DeleteInboxMessage(ctx context.Context, id string) error {
	_, err := c.c.DeleteInboxMessage(ctx, &pb.DeleteInboxMessageRequest{
		Id: id,
	})
	return err
}

// DeleteSentboxMessage deletes a sent message by ID.
func (c *Client) DeleteSentboxMessage(ctx context.Context, id string) error {
	_, err := c.c.DeleteSentboxMessage(ctx, &pb.DeleteSentboxMessageRequest{
		Id: id,
	})
	return err
}
