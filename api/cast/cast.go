package cast

import (
	pb "github.com/textileio/go-mail/api/pb/mail"
	"github.com/textileio/go-mail/collection"
)

func InboxMessagesToPb(messages []*collection.InboxMessage) []*pb.Message {
	pmsg := make([]*pb.Message, len(messages))
	for idx, msg := range messages {
		pmsg[idx] = &pb.Message{
			Id:        msg.ID,
			From:      msg.From,
			To:        msg.To,
			Body:      []byte(msg.Body),
			Signature: []byte(msg.Signature),
			ReadAt:    msg.ReadAt,
		}
	}

	return pmsg
}

func SentboxMessagesToPb(messages []*collection.SentboxMessage) []*pb.Message {
	pmsg := make([]*pb.Message, len(messages))
	for idx, msg := range messages {
		pmsg[idx] = &pb.Message{
			Id:        msg.ID,
			From:      msg.From,
			To:        msg.To,
			Body:      []byte(msg.Body),
			Signature: []byte(msg.Signature),
		}
	}

	return pmsg
}
