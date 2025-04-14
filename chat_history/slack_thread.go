package chat_history

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shaharia-lab/goai"

	"github.com/slack-go/slack"
)

// SlackClient interface defines the methods we need from the slack SDK
type SlackClient interface {
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	GetConversationReplies(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error)
	GetConversationHistory(params *slack.GetConversationHistoryParameters) (*slack.GetConversationHistoryResponse, error)
	DeleteMessage(channel string, messageTimestamp string) (string, string, error)
}

// ChatInitializer represents a function that initializes a chat conversation
type ChatInitializer func(ctx context.Context, channelID string, client SlackClient) (string, error)

// DefaultChatInitializer creates a simple message to start a thread
func DefaultChatInitializer(ctx context.Context, channelID string, client SlackClient) (string, error) {
	initialMsg := "New conversation started"
	timestamp, _, err := client.PostMessage(
		channelID,
		slack.MsgOptionText(initialMsg, false),
	)
	return timestamp, err
}

// SlackStorageConfig holds the configuration for SlackChatHistoryStorage
type SlackStorageConfig struct {
	AppID             string
	ClientID          string
	ClientSecret      string
	SlackSigninSecret string
	BotUserID         string
	ChannelID         string
	BotToken          string
	ChatInitFn        ChatInitializer
}

// SlackChatHistoryStorage implements ChatHistoryStorage using Slack threads
type SlackChatHistoryStorage struct {
	client          SlackClient
	config          SlackStorageConfig
	channelID       string
	botUserID       string
	chatInitializer ChatInitializer
}

// NewSlackChatHistoryStorage creates a new instance of SlackChatHistoryStorage
func NewSlackChatHistoryStorage(config SlackStorageConfig) (*SlackChatHistoryStorage, error) {
	if config.BotToken == "" {
		return nil, errors.New("bot token is required")
	}

	if config.ChannelID == "" {
		return nil, errors.New("channel ID is required")
	}

	if config.BotUserID == "" {
		return nil, errors.New("bot user ID is required")
	}

	slackClient := slack.New(config.BotToken)

	// Use the provided initializer or the default one
	chatInitFn := config.ChatInitFn
	if chatInitFn == nil {
		chatInitFn = DefaultChatInitializer
	}

	return &SlackChatHistoryStorage{
		client:          slackClient,
		config:          config,
		channelID:       config.ChannelID,
		botUserID:       config.BotUserID,
		chatInitializer: chatInitFn,
	}, nil
}

// RegisterThread allows registering an existing thread timestamp as a chat session
func (s *SlackChatHistoryStorage) RegisterThread(ctx context.Context, timestamp string) (*ChatHistory, error) {
	// Verify the thread exists
	params := &slack.GetConversationRepliesParameters{
		ChannelID: s.channelID,
		Timestamp: timestamp,
	}

	messages, _, _, err := s.client.GetConversationReplies(params)
	if err != nil {
		return nil, fmt.Errorf("failed to verify thread existence: %w", err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no thread found with timestamp: %s", timestamp)
	}

	createdAt, err := parseSlackTimestamp(timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	chatHistory := &ChatHistory{
		SessionID: timestamp,
		Messages:  []ChatHistoryMessage{},
		CreatedAt: createdAt,
	}

	return chatHistory, nil
}

// CreateChat initializes a new chat conversation using the configured initializer
func (s *SlackChatHistoryStorage) CreateChat(ctx context.Context) (*ChatHistory, error) {
	// Use the configured initializer to create a new thread
	timestamp, err := s.chatInitializer(ctx, s.channelID, s.client)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chat: %w", err)
	}

	// Use the timestamp as the session ID
	chatHistory := &ChatHistory{
		SessionID: timestamp,
		Messages:  []ChatHistoryMessage{},
		CreatedAt: time.Now(),
	}

	return chatHistory, nil
}

// AddMessage adds a new message to an existing conversation thread
func (s *SlackChatHistoryStorage) AddMessage(ctx context.Context, sessionID string, message ChatHistoryMessage) error {
	_, _, err := s.client.PostMessage(
		s.channelID,
		slack.MsgOptionText(message.Text, false),
		slack.MsgOptionTS(sessionID),
	)

	if err != nil {
		return fmt.Errorf("failed to add message to thread: %w", err)
	}

	return nil
}

// GetChat retrieves a conversation by its thread timestamp (SessionID)
func (s *SlackChatHistoryStorage) GetChat(ctx context.Context, sessionID string) (*ChatHistory, error) {
	params := &slack.GetConversationRepliesParameters{
		ChannelID: s.channelID,
		Timestamp: sessionID,
	}

	messages, _, _, err := s.client.GetConversationReplies(params)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation replies: %w", err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages found for session ID: %s", sessionID)
	}

	chatMessages := make([]ChatHistoryMessage, 0, len(messages))
	var createdAt time.Time

	for i, msg := range messages {
		if i == 0 {
			ts, err := parseSlackTimestamp(msg.Timestamp)
			if err != nil {
				return nil, fmt.Errorf("failed to parse timestamp: %w", err)
			}
			createdAt = ts
			continue
		}

		ts, err := parseSlackTimestamp(msg.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}

		role := goai.UserRole
		if msg.User == s.botUserID {
			role = goai.AssistantRole
		}

		chatMessages = append(chatMessages, ChatHistoryMessage{
			LLMMessage: goai.LLMMessage{
				Role: role,
				Text: msg.Text,
			},
			GeneratedAt: ts,
		})
	}

	return &ChatHistory{
		SessionID: sessionID,
		Messages:  chatMessages,
		CreatedAt: createdAt,
	}, nil
}

// ListChatHistories returns all thread conversations started by the bot in the channel
func (s *SlackChatHistoryStorage) ListChatHistories(ctx context.Context) ([]ChatHistory, error) {
	params := &slack.GetConversationHistoryParameters{
		ChannelID: s.channelID,
		Limit:     100,
	}

	resp, err := s.client.GetConversationHistory(params)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	var histories []ChatHistory

	for _, msg := range resp.Messages {
		if msg.User == s.botUserID && msg.ThreadTimestamp == "" && msg.ReplyCount > 0 {
			_, err := parseSlackTimestamp(msg.Timestamp)
			if err != nil {
				continue
			}

			chat, err := s.GetChat(ctx, msg.Timestamp)
			if err != nil {
				continue
			}

			histories = append(histories, *chat)
		}
	}

	return histories, nil
}

// DeleteChat removes a conversation thread
func (s *SlackChatHistoryStorage) DeleteChat(ctx context.Context, sessionID string) error {
	_, _, err := s.client.DeleteMessage(s.channelID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete chat thread: %w", err)
	}

	return nil
}

func parseSlackTimestamp(timestamp string) (time.Time, error) {
	var sec, nsec int64
	_, err := fmt.Sscanf(timestamp, "%d.%d", &sec, &nsec)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid slack timestamp format: %s", timestamp)
	}

	nsec = nsec * 1000000

	return time.Unix(sec, nsec), nil
}
