package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/internal/utils"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type UserRepository struct {
	dbPool *pgxpool.Pool

	logger logging.Logger
}

func NewUserRepository(dbPool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		dbPool: dbPool,
		logger: logging.GetLogger(),
	}
}

func (r *UserRepository) List(ctx context.Context) ([]*domain.User, error) {
	query := `
		SELECT 
			id, username, password, 
			first_name, last_name, email, 
			birth_date::text, department, is_deleted, 
			created_at, updated_at
		FROM users
		WHERE is_deleted IS FALSE
	`

	rows, err := r.dbPool.Query(ctx, query)
	if err != nil {
		r.logger.WithError(err).Error("Unable to list users from database")

		return nil, err
	}
	defer rows.Close()

	users := make([]*domain.User, 0)

	for rows.Next() {
		user := new(domain.User)
		updatedAt := new(sql.NullTime)

		if err = rows.Scan(
			&user.ID, &user.Username, &user.Password,
			&user.FirstName, &user.LastName, &user.Email,
			&user.BirthDate, &user.Department, &user.IsDeleted,
			&user.CreatedAt, updatedAt,
		); err != nil {
			r.logger.WithError(err).Error("Unable to scan user")

			return nil, err
		}

		if updatedAt.Valid {
			user.UpdatedAt = &updatedAt.Time
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		r.logger.WithError(err).Error("Error occurred while reading users")

		return nil, err
	}

	return users, nil
}

func (r *UserRepository) Create(ctx context.Context, dto domain.CreateUserDTO) (*domain.User, error) {
	id := ""
	createdAt := new(sql.NullTime)
	query := `
		INSERT INTO users(
			username, password, first_name, 
			last_name, email, birth_date, department
		) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at
	`

	if err := r.dbPool.QueryRow(
		ctx, query,
		dto.Username, dto.Password, dto.FirstName,
		dto.LastName, dto.Email, dto.BirthDate, dto.Department,
	).Scan(&id, createdAt); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			r.logger.WithError(err).Debug("user with such fields is already exist")

			return nil, domain.ErrUserUniqueViolation
		}

		r.logger.WithError(err).Error("Error occurred while creating user into the database")

		return nil, err
	}

	user := &domain.User{
		ID:         id,
		Username:   dto.Username,
		Password:   dto.Password,
		Email:      dto.Email,
		FirstName:  dto.FirstName,
		LastName:   dto.LastName,
		BirthDate:  dto.BirthDate,
		Department: dto.Department,
	}
	if createdAt.Valid {
		user.CreatedAt = &createdAt.Time
	}

	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	if !utils.IsValidUUID(id) {
		r.logger.Debugf("user is not found with id = %s", id)

		return nil, domain.ErrUserNotFound
	}

	return r.getBy(ctx, "id", id)
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	return r.getBy(ctx, "username", username)
}

func (r *UserRepository) getBy(ctx context.Context, fieldName string, fieldValue interface{}) (*domain.User, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, username, password, 
			first_name, last_name, email, 
			birth_date::text, department, is_deleted, 
			created_at, updated_at
		FROM users WHERE %s = $1 AND is_deleted IS FALSE`,
		fieldName,
	)

	user := new(domain.User)
	updatedAt := new(sql.NullTime)

	row := r.dbPool.QueryRow(ctx, query, fieldValue)
	if err := row.Scan(
		&user.ID, &user.Username, &user.Password,
		&user.FirstName, &user.LastName, &user.Email,
		&user.BirthDate, &user.Department, &user.IsDeleted,
		&user.CreatedAt, updatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Debugf("user is not found with %s = %s", fieldName, fieldValue)

			return nil, domain.ErrUserNotFound
		}

		r.logger.WithError(err).Errorf("Error occurred while getting user by %s", fieldName)

		return nil, nil
	}

	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}

	return user, nil
}

func (r *UserRepository) Update(ctx context.Context, dto domain.UpdateUserDTO) (*domain.User, error) {
	if !utils.IsValidUUID(dto.ID) {
		r.logger.Debugf("user is not found with id = %s", dto.ID)

		return nil, domain.ErrUserNotFound
	}

	isDeleted := false
	createdAt := new(sql.NullTime)
	updatedAt := new(sql.NullTime)

	query := `
		UPDATE users SET
			username = $2, password = $3, 
			first_name = $4, last_name = $5, email = $6, 
			birth_date = $7, department = $8
		WHERE id = $1 AND is_deleted IS FALSE
		RETURNING is_deleted, created_at, updated_at
	`

	if err := r.dbPool.QueryRow(
		ctx, query,
		dto.ID,
		dto.Username, dto.Password,
		dto.FirstName, dto.LastName, dto.Email,
		dto.BirthDate, dto.Department,
	).Scan(&isDeleted, createdAt, updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Debugf("user is not found with id = %s", dto.ID)

			return nil, domain.ErrUserNotFound
		}

		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			r.logger.WithError(err).Debug("user with such fields is already exist")

			return nil, domain.ErrUserUniqueViolation
		}

		r.logger.WithError(err).Error("Error occurred while updating user into the database")

		return nil, err
	}

	user := &domain.User{
		ID:         dto.ID,
		Username:   dto.Username,
		Password:   dto.Password,
		Email:      dto.Email,
		FirstName:  dto.FirstName,
		LastName:   dto.LastName,
		BirthDate:  dto.BirthDate,
		Department: dto.Department,
		IsDeleted:  isDeleted,
	}
	if createdAt.Valid {
		user.CreatedAt = &createdAt.Time
	}

	if updatedAt.Valid {
		user.UpdatedAt = &updatedAt.Time
	}

	return user, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	if !utils.IsValidUUID(id) {
		r.logger.Debugf("user is not found with id = %s", id)

		return domain.ErrUserNotFound
	}

	query := `
		UPDATE users SET
			is_deleted = TRUE
		WHERE id = $1 AND is_deleted IS FALSE
	`

	cmgTag, err := r.dbPool.Exec(ctx, query, id)
	if err != nil {
		r.logger.WithError(err).Error("Error occurred while updating user into the database")

		return err
	}

	if cmgTag.RowsAffected() == 0 {
		r.logger.Debugf("user is not found with id = %s", id)

		return domain.ErrUserNotFound
	}

	return nil
}
