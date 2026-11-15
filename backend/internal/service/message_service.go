package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"vuathau/backend/internal/model"
	"vuathau/backend/internal/repository"
)

const maxMessageLength = 2000

// Notifier pushes real-time events to connected users (implemented by websocket.Hub).
type Notifier interface {
	SendToUser(userID int64, event any)
}

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type SendMessageInput struct {
	ReceiverID int64  `json:"receiver_id"`
	Content    string `json:"content"`
}

type MessageService interface {
	Send(ctx context.Context, senderID int64, in SendMessageInput) (*model.Message, error)
	ListConversation(ctx context.Context, userID, otherID int64, limit, offset int) ([]model.Message, error)
}

type messageService struct {
	repo     repository.MessageRepository
	notifier Notifier
}

// NewMessageService creates the service. notifier may be nil to disable real-time push.
func NewMessageService(repo repository.MessageRepository, notifier Notifier) MessageService {
	return &messageService{repo: repo, notifier: notifier}
}

func (s *messageService) Send(ctx context.Context, senderID int64, in SendMessageInput) (*model.Message, error) {
	content := strings.TrimSpace(in.Content)
	if content == "" || utf8.RuneCountInString(content) > maxMessageLength {
		return nil, fmt.Errorf("%w: content must be 1-%d characters", ErrInvalidInput, maxMessageLength)
	}
	if in.ReceiverID <= 0 || in.ReceiverID == senderID {
		return nil, fmt.Errorf("%w: receiver_id is not valid", ErrInvalidInput)
	}

	m := &model.Message{SenderID: senderID, ReceiverID: in.ReceiverID, Content: content}
	if err := s.repo.Create(ctx, m); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("%w: receiver does not exist", ErrNotFound)
		}
		return nil, fmt.Errorf("create message: %w", err)
	}

	if s.notifier != nil {
		evt := Event{Type: "message:new", Data: m}
		s.notifier.SendToUser(m.ReceiverID, evt)
		s.notifier.SendToUser(m.SenderID, evt) // sync sender's other tabs/devices
	}

	return m, nil
}

func (s *messageService) ListConversation(ctx context.Context, userID, otherID int64, limit, offset int) ([]model.Message, error) {
	limit, offset = normalizePage(limit, offset)
	return s.repo.ListConversation(ctx, userID, otherID, limit, offset)
}
