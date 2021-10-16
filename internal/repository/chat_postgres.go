package repository

import (
	"context"
	"errors"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/internal/utils"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/jackc/pgx/v4"
)

type chatPostgresRepository struct {
	dbPool PgxPool
	logger logging.Logger
}

func NewChatPostgresRepository(dbPool PgxPool) ChatRepository {
	return &chatPostgresRepository{
		dbPool: dbPool,
		logger: logging.GetLogger(),
	}
}

func (r *chatPostgresRepository) List(ctx context.Context, memberID string) ([]domain.Chat, error) {
	query := `SELECT 
		id, name, description, 
		creator_id, created_at, updated_at 
	FROM chats 
	INNER JOIN users_chats 
		ON chats.id = users_chats.chat_id
	WHERE users_chats.user_id = $1`

	rows, err := r.dbPool.Query(ctx, query, memberID)
	if err != nil {
		r.logger.WithError(err).Error("Unable to list chats from database")
		return nil, err
	}
	defer rows.Close()

	chats := make([]domain.Chat, 0)

	for rows.Next() {
		var chat domain.Chat

		if err = rows.Scan(
			&chat.ID, &chat.Name, &chat.Description,
			&chat.CreatorID, &chat.CreatedAt, &chat.UpdatedAt,
		); err != nil {
			r.logger.WithError(err).Error("Unable to scan chat")
			return nil, err
		}

		chats = append(chats, chat)
	}

	if err = rows.Err(); err != nil {
		r.logger.WithError(err).Error("An error occurred while reading chats")
		return nil, err
	}

	return chats, nil
}

func (r *chatPostgresRepository) Create(ctx context.Context, dto domain.CreateChatDTO) (domain.Chat, error) {
	var chat domain.Chat

	err := r.dbPool.BeginFunc(ctx, func(tx pgx.Tx) error {
		query := "INSERT INTO chats (name, description, creator_id) VALUES ($1, $2, $3) RETURNING id, created_at"

		if err := r.dbPool.QueryRow(
			ctx, query,
			dto.Name, dto.Description, dto.CreatorID,
		).Scan(&chat.ID, &chat.CreatedAt); err != nil {
			r.logger.WithError(err).Error("An error occurred while creating chat into the database")
			return err
		}

		query = "INSERT INTO users_chats (user_id, chat_id) VALUES ($1, $2)"

		if _, err := r.dbPool.Exec(ctx, query, dto.CreatorID, chat.ID); err != nil {
			r.logger.WithError(err).Error("An error occurred while inserting into users_chats table")
			return err
		}

		return nil
	})
	if err != nil {
		return domain.Chat{}, err
	}

	chat.Name = dto.Name
	chat.Description = dto.Description
	chat.CreatorID = dto.CreatorID

	return chat, nil
}

func (r *chatPostgresRepository) GetByID(ctx context.Context, chatID, memberID string) (domain.Chat, error) {
	if !utils.IsValidUUID(chatID) || !utils.IsValidUUID(memberID) {
		r.logger.Debugf("chat is not found with chatID = %s, memberID = %s", chatID, memberID)
		return domain.Chat{}, domain.ErrChatNotFound
	}

	query := `SELECT 
		id, name, description, 
		creator_id, created_at, updated_at 
	FROM chats 
	INNER JOIN users_chats 
		ON chats.id = users_chats.chat_id
	WHERE chats.id = $1 AND users_chats.user_id = $2`

	var chat domain.Chat

	if err := r.dbPool.QueryRow(ctx, query, chatID, memberID).Scan(
		&chat.ID, &chat.Name, &chat.Description,
		&chat.CreatorID, &chat.CreatedAt, &chat.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Debugf("chat is not found with chatID = %s, memberID = %s", chatID, memberID)
			return domain.Chat{}, domain.ErrChatNotFound
		}

		r.logger.WithError(err).Error("An error occurred while getting chat by id")

		return domain.Chat{}, err
	}

	return chat, nil
}

func (r *chatPostgresRepository) Update(ctx context.Context, dto domain.UpdateChatDTO) (domain.Chat, error) {
	if !utils.IsValidUUID(dto.ID) || !utils.IsValidUUID(dto.CreatorID) {
		r.logger.Debugf("chat is not found with chatID = %s, creatorID = %s", dto.ID, dto.CreatorID)
		return domain.Chat{}, domain.ErrChatNotFound
	}

	query := `UPDATE chats 
	SET name = $1, description = $2 
	WHERE id = $3 AND creator_id = $4 
	RETURNING created_at, updated_at`

	var chat domain.Chat

	if err := r.dbPool.QueryRow(
		ctx, query,
		dto.Name, dto.Description,
		dto.ID, dto.CreatorID,
	).Scan(&chat.CreatedAt, &chat.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Debugf("chat is not found with chatID = %s, creatorID = %s", dto.ID, dto.CreatorID)
			return domain.Chat{}, domain.ErrChatNotFound
		}

		r.logger.WithError(err).Error("An error occurred while updating chat into the database")

		return domain.Chat{}, err
	}

	chat.ID = dto.ID
	chat.Name = dto.Name
	chat.Description = dto.Description
	chat.CreatorID = dto.CreatorID

	return chat, nil
}

func (r *chatPostgresRepository) Delete(ctx context.Context, chatID, creatorID string) error {
	if !utils.IsValidUUID(chatID) || !utils.IsValidUUID(creatorID) {
		r.logger.Debugf("chat is not found with chatID = %s, creatorID = %s", chatID, creatorID)
		return domain.ErrChatNotFound
	}

	query := "DELETE FROM chats WHERE id = $1 AND creator_id = $2"

	cmgTag, err := r.dbPool.Exec(ctx, query, chatID, creatorID)
	if err != nil {
		r.logger.WithError(err).Error("An error occurred while deleting chat from database")
		return err
	}

	if cmgTag.RowsAffected() == 0 {
		r.logger.Debugf("chat is not found with chatID = %s, creatorID = %s", chatID, creatorID)
		return domain.ErrChatNotFound
	}

	return nil
}