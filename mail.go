package mail

import (
	"context"
	"errors"
	"fmt"

	pb "github.com/textileio/go-mail/api/pb/mail"
	"github.com/textileio/go-mail/collection"
	dbc "github.com/textileio/go-threads/api/client"
	"github.com/textileio/go-threads/core/thread"
	nc "github.com/textileio/go-threads/net/api/client"
)

const (
	defaultMessagePageSize = 100
	maxMessagePageSize     = 10000
	minMessageReadAt       = float64(0)
)

var (
	ErrMailBoxNotFound = errors.New("mail not found")
)

type Mail struct {
	m   *collection.Mail
	net *nc.Client
	db  *dbc.Client
}

func NewMail(
	tc *dbc.Client,
	net *nc.Client,
) (*Mail, error) {
	m, err := collection.NewMail(tc)
	if err != nil {
		return nil, fmt.Errorf("getting mail collection: %v", err)
	}
	return &Mail{
		m:   m,
		net: net,
		db:  tc,
	}, nil
}

func (m *Mail) Close() error {
	m.net.Close()
	m.db.Close()
	return nil
}

func (m *Mail) SetupMailbox(ctx context.Context, req *pb.SetupMailboxRequest) (*pb.SetupMailboxResponse, error) {

	box, err := m.getOrCreateMailbox(ctx, thread.NewLibp2pPubKey())
	if err != nil {
		return nil, err
	}

	return &pb.SetupMailboxResponse{
		MailboxId: []byte(box),
	}, nil
}

func (m *Mail) getOrCreateMailbox(ctx context.Context, key thread.PubKey, opts ...collection.Option) (thread.ID, error) {
	id, err := m.m.NewMailbox(ctx)
	if errors.Is(err, collection.ErrMailboxExists) {
		// thrd, err :=
	}

	return id, nil
}
