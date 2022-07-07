package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/textileio/go-mail"
	"github.com/textileio/go-mail/api/cast"
	pb "github.com/textileio/go-mail/api/pb/mail"
	"github.com/textileio/go-threads/core/did"
	core "github.com/textileio/go-threads/core/thread"
)

type Service struct {
	lib *mail.Mail
}

var _ pb.APIServiceServer = (*Service)(nil)

func NewService(lib *mail.Mail) *Service {
	return &Service{
		lib: lib,
	}
}

const (
	defaultMessagePageSize = 100
	maxMessagePageSize     = 10000
	minMessageReadAt       = float64(0)
)

var (
	// ErrMailboxNotFound indicates that a mailbox has not been setup for a mail sender/receiver.
	ErrMailboxNotFound = errors.New("mail not found")
)

func (s *Service) SendMessage(ctx context.Context, in *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	thread, identity, err := getThreadAndIdentity(ctx, in.Thread)
	if err != nil {
		return nil, err
	}
	id, createAt, err := s.lib.SendMessage(ctx, thread, identity, in.To, in.ToBody, in.ToSignature, in.FromBody, in.FromSignature)
	if err != nil {
		return nil, err
	}

	return &pb.SendMessageResponse{
		Id:        id,
		CreatedAt: createAt,
	}, nil
}

func (s *Service) ListInboxMessages(ctx context.Context, in *pb.ListInboxMessagesRequest) (*pb.ListInboxMessagesResponse, error) {
	thread, identity, err := getThreadAndIdentity(ctx, in.Thread)
	if err != nil {
		return nil, err
	}
	messages, err := s.lib.ListInboxMessages(ctx, thread, identity, in.Seek, in.Limit, in.Ascending, in.Status)
	if err != nil {
		return nil, err
	}

	return &pb.ListInboxMessagesResponse{
		Messages: cast.InboxMessagesToPb(messages),
	}, nil
}

func (s *Service) ListSentboxMessages(ctx context.Context, in *pb.ListSentboxMessagesRequest) (*pb.ListSentboxMessagesResponse, error) {
	thread, identity, err := getThreadAndIdentity(ctx, in.Thread)
	if err != nil {
		return nil, err
	}
	messages, err := s.lib.ListSentboxMessages(ctx, thread, identity, in.Seek, in.Limit, in.Ascending, in.Status)
	if err != nil {
		return nil, err
	}

	return &pb.ListSentboxMessagesResponse{
		Messages: cast.SentboxMessagesToPb(messages),
	}, nil
}

func (s *Service) ReadInboxMessage(ctx context.Context, in *pb.ReadInboxMessageRequest) (*pb.ReadInboxMessageResponse, error) {
	thread, identity, err := getThreadAndIdentity(ctx, in.Thread)
	if err != nil {
		return nil, err
	}

	readAt, err := s.lib.ReadInboxMessage(ctx, thread, identity, in.Id)
	if err != nil {
		return nil, err
	}

	return &pb.ReadInboxMessageResponse{
		ReadAt: readAt,
	}, nil
}

func (s *Service) DeleteInboxMessage(ctx context.Context, in *pb.DeleteInboxMessageRequest) (*pb.DeleteInboxMessageResponse, error) {
	thread, identity, err := getThreadAndIdentity(ctx, in.Thread)
	if err != nil {
		return nil, err
	}

	if err = s.lib.DeleteInboxMessage(ctx, thread, identity, in.Id); err != nil {
		return nil, err
	}

	return &pb.DeleteInboxMessageResponse{}, nil
}

func (s *Service) DeleteSentboxMessage(ctx context.Context, in *pb.DeleteSentboxMessageRequest) (*pb.DeleteSentboxMessageResponse, error) {
	thread, identity, err := getThreadAndIdentity(ctx, in.Thread)
	if err != nil {
		return nil, err
	}

	if err = s.lib.DeleteSentboxMessage(ctx, thread, identity, in.Id); err != nil {
		return nil, err
	}

	return &pb.DeleteSentboxMessageResponse{}, nil
}

func getThreadAndIdentity(ctx context.Context, threadStr string) (thread core.ID, identity did.Token, err error) {
	if len(threadStr) != 0 {
		thread, err = core.Decode(threadStr)
		if err != nil {
			return "", "", fmt.Errorf("decoding thread: %v", err)
		}
	}
	identity, err = did.NewTokenFromMD(ctx)
	if err != nil {
		return "", "", fmt.Errorf("getting identity token: %v", err)
	}
	return thread, identity, nil
}
