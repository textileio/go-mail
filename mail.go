package mail

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/gogo/status"
	logging "github.com/ipfs/go-log/v2"
	ulid "github.com/oklog/ulid/v2"
	pb "github.com/textileio/go-mail/api/pb/mail"
	"github.com/textileio/go-mail/collection"
	dbc "github.com/textileio/go-threads/api/client"
	coredb "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/did"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/db"
	nc "github.com/textileio/go-threads/net/api/client"
	"google.golang.org/grpc/codes"
)

var (
	log = logging.Logger("buckets")

	// GatewayURL is used to construct externally facing bucket links.
	GatewayURL string

	// ThreadsGatewayURL is used to construct externally facing bucket links.
	ThreadsGatewayURL string

	// WWWDomain can be set to specify the domain to use for bucket website hosting, e.g.,
	// if this is set to mydomain.com, buckets can be rendered as a website at the following URL:
	//   https://<bucket_key>.mydomain.com
	WWWDomain string
)

const (
	defaultMessagePageSize = 100
	maxMessagePageSize     = 10000
	minMessageReadAt       = float64(0)
)

var (
	ErrMailboxNotFound = errors.New("mail not found")
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

func (m *Mail) SendMessage(
	ctx context.Context,
	thrd thread.ID,
	token did.Token, // JWT authentication (from a did.DID)
	to string,
	toBody, toSignature, fromBody, fromSignature []byte) (string, int64, error) {
	if err := thrd.Validate(); err != nil {
		return "", -1, fmt.Errorf("invalid thread id: %v", err)
	}

	toPubKey := &thread.Libp2pPubKey{}
	if err := toPubKey.UnmarshalString(to); err != nil {
		return "", -1, status.Error(codes.FailedPrecondition, "Invalid public key")
	}
	if ok, err := toPubKey.Verify(toBody, toSignature); !ok || err != nil {
		return "", -1, status.Error(codes.Unauthenticated, "Bad message signature")
	}

	var inbox thread.ID
	err := func() error {
		var err error
		defer func() {
			if e := recover(); e != nil {
				err = fmt.Errorf("new pubkey id err:%v", e)
			}
		}()
		inbox = thread.NewPubKeyIDV1(toPubKey)
		return err
	}()
	if err != nil {
		return "", -1, err
	}

	identity := thrd.DID()
	d, _ := identity.Decode()
	from := d.ID
	fromPubKey := &thread.Libp2pPubKey{}
	if err := fromPubKey.UnmarshalString(from); err != nil {
		return "", -1, status.Error(codes.FailedPrecondition, "Invalid public key")
	}
	if ok, err := fromPubKey.Verify(fromBody, fromSignature); !ok || err != nil {
		return "", -1, status.Error(codes.Unauthenticated, "Bad message signature")
	}

	msgID := coredb.NewInstanceID().String()
	now := time.Now().UnixNano()

	toMsg := collection.InboxMessage{
		ID:        msgID,
		From:      string(identity),
		To:        string(did.NewKeyDID(toPubKey.String())),
		Body:      base64.StdEncoding.EncodeToString(toBody),
		Signature: base64.StdEncoding.EncodeToString(toSignature),
		CreatedAt: now,
	}

	if _, err := m.m.Inbox.Create(ctx, inbox, toMsg, collection.WithIdentity(token)); err != nil {
		return "", -1, err
	}

	fromMsg := collection.SentboxMessage{
		ID:        msgID,
		From:      string(identity),
		To:        string(did.NewKeyDID(toPubKey.String())),
		Body:      base64.StdEncoding.EncodeToString(fromBody),
		Signature: base64.StdEncoding.EncodeToString(fromSignature),
		CreatedAt: now,
	}
	if _, err := m.m.Sentbox.Create(ctx, thrd, fromMsg, collection.WithIdentity(token)); err != nil {
		return "", -1, err
	}
	return msgID, now, nil
}

func (m *Mail) ListInboxMessages(ctx context.Context, thrd thread.ID, identity did.Token, seek string, limit int64,
	ascending bool, stat pb.MailboxMessageStatus) ([]*collection.InboxMessage, error) {
	query, err := getMailboxQuery(seek, limit, ascending, stat)
	if err != nil {
		return nil, err
	}
	res, err := m.m.Inbox.List(ctx, thrd, query, collection.WithIdentity(identity))
	if err != nil {
		return nil, err
	}

	list := res.([]*collection.InboxMessage)
	return list, nil
}

func (m *Mail) ListSentboxMessages(ctx context.Context, thrd thread.ID, identity did.Token, seek string, limit int64,
	ascending bool, stat pb.MailboxMessageStatus) ([]*collection.SentboxMessage, error) {
	query, err := getMailboxQuery(seek, limit, ascending, stat)
	if err != nil {
		return nil, err
	}
	res, err := m.m.Sentbox.List(ctx, thrd, query, collection.WithIdentity(identity))
	if err != nil {
		return nil, err
	}

	list := res.([]*collection.SentboxMessage)
	return list, nil
}

func getMailboxQuery(seek string, limit int64, asc bool, stat pb.MailboxMessageStatus) (q *db.Query, err error) {
	if asc {
		q = db.OrderByID()
		if seek != "" {
			q.SeekID(coredb.InstanceID(seek))
		}
	} else {
		q = db.OrderByIDDesc()
		if seek == "" {
			seek = ulid.MustNew(ulid.MaxTime(), rand.Reader).String()
		}
		q.SeekID(coredb.InstanceID(seek))
	}
	if limit == 0 {
		limit = defaultMessagePageSize
	} else if limit > maxMessagePageSize {
		limit = maxMessagePageSize
	}
	q.LimitTo(int(limit))
	switch stat {
	case pb.MailboxMessageStatus_ALL:
	case pb.MailboxMessageStatus_UNSPECIFIED:
		break
	case pb.MailboxMessageStatus_READ:
		q.And("read_at").Gt(minMessageReadAt)
	case pb.MailboxMessageStatus_UNREAD:
		q.And("read_at").Eq(minMessageReadAt)
	default:
		return nil, fmt.Errorf("unknown message status: %v", stat.String())
	}
	return q, nil
}

func (m *Mail) ReadInboxMessage(ctx context.Context, thrd thread.ID, identity did.Token, id string) (int64, error) {
	msg := &collection.InboxMessage{}
	err := m.m.Inbox.Get(ctx, thrd, id, msg, collection.WithIdentity(identity))
	if err != nil {
		return -1, err
	}

	return msg.ReadAt, nil
}

func (m *Mail) DeleteInboxMessage(ctx context.Context, thrd thread.ID, identity did.Token, id string) error {
	if err := m.m.Inbox.Delete(ctx, thrd, id, collection.WithIdentity(identity)); err != nil {
		return err
	}

	return nil
}

func (m *Mail) DeleteSentboxMessage(ctx context.Context, thrd thread.ID, identity did.Token, id string) error {
	if err := m.m.Sentbox.Delete(ctx, thrd, id, collection.WithIdentity(identity)); err != nil {
		return err
	}

	return nil
}
