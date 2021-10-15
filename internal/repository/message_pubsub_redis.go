package repository

import (
	"context"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/go-redis/redis/v8"
)

type messageRedisSubscriber struct {
	pubSub *redis.PubSub
	logger logging.Logger
}

func (s *messageRedisSubscriber) ReceiveMessage(ctx context.Context) (domain.Message, error) {
	msg, err := s.pubSub.ReceiveMessage(ctx)
	if err != nil {
		s.logger.WithError(err).Error("An error occurred while receiving message from pubSub")
		return domain.Message{}, err
	}

	var message domain.Message
	if err = message.Decode([]byte(msg.Payload)); err != nil {
		s.logger.WithError(err).Error("An error occurred while unmarshalling the message")
		return domain.Message{}, err
	}

	return message, nil
}

func (s *messageRedisSubscriber) MessageChannel(ctx context.Context) <-chan domain.Message {
	ch := s.pubSub.Channel()
	msgCh := make(chan domain.Message)

	go func() {
		defer close(msgCh)

		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}

				var message domain.Message
				if err := message.Decode([]byte(msg.Payload)); err != nil {
					s.logger.WithError(err).Error("An error occurred while unmarshalling the message")
					return
				}

				msgCh <- message
			}
		}
	}()

	return msgCh
}

func (s *messageRedisSubscriber) Unsubscribe(ctx context.Context, topics ...string) error {
	if err := s.pubSub.Unsubscribe(ctx, topics...); err != nil {
		s.logger.WithError(err).Errorf("An error occurred while unsubscribing to topics %v", topics)
		return err
	}

	return nil
}

func (s *messageRedisSubscriber) Subscribe(ctx context.Context, topics ...string) error {
	if err := s.pubSub.Subscribe(ctx, topics...); err != nil {
		s.logger.WithError(err).Errorf("An error occurred while subscribing to topics %v", topics)
		return err
	}

	return nil
}

func (s *messageRedisSubscriber) Close() error {
	if err := s.pubSub.Close(); err != nil {
		s.logger.WithError(err).Error("An error occurred while closing pubSub subscriber")
		return err
	}

	return nil
}

type messageRedisPubSub struct {
	redisClient *redis.Client
	logger      logging.Logger
}

func NewMessagePubSub(redisClient *redis.Client) MessagePubSub {
	return &messageRedisPubSub{
		redisClient: redisClient,
		logger:      logging.GetLogger(),
	}
}

func (ps *messageRedisPubSub) Publish(ctx context.Context, message domain.Message, topic string) error {
	payload, err := message.Encode()
	if err != nil {
		ps.logger.WithError(err).Error("An error occurred while marshaling the message")
		return err
	}

	if err = ps.redisClient.Publish(ctx, topic, payload).Err(); err != nil {
		ps.logger.WithError(err).Error("An error occurred while publishing the message")
		return err
	}

	return nil
}

func (ps *messageRedisPubSub) Subscribe(ctx context.Context, topics ...string) MessageSubscriber {
	return &messageRedisSubscriber{
		logger: ps.logger,
		pubSub: ps.redisClient.Subscribe(ctx, topics...),
	}
}
