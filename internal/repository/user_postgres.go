package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Mort4lis/scht-backend/internal/domain"
	"github.com/Mort4lis/scht-backend/internal/utils"
	"github.com/Mort4lis/scht-backend/pkg/logging"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
)

type userPostgresRepository struct {
	dbPool PgxPool
	logger logging.Logger
}

func NewUserPostgresRepository(dbPool PgxPool) UserRepository {
	return &userPostgresRepository{
		dbPool: dbPool,
		logger: logging.GetLogger(),
	}
}

func (r *userPostgresRepository) List(ctx context.Context) ([]domain.User, error) {
	query := `SELECT 
		id, username, password, 
		first_name, last_name, email, 
		birth_date, department, is_deleted, 
		created_at, updated_at
	FROM users`

	rows, err := r.dbPool.Query(ctx, query)
	if err != nil {
		r.logger.WithError(err).Error("Unable to list users from database")
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)

	for rows.Next() {
		var (
			user      domain.User
			birthDate pgtype.Date
		)

		if err = rows.Scan(
			&user.ID, &user.Username, &user.Password,
			&user.FirstName, &user.LastName, &user.Email,
			&birthDate, &user.Department, &user.IsDeleted,
			&user.CreatedAt, &user.UpdatedAt,
		); err != nil {
			r.logger.WithError(err).Error("Unable to scan user")
			return nil, err
		}

		if birthDate.Status == pgtype.Present {
			user.BirthDate = birthDate.Time.Format("2006-01-02")
		}

		users = append(users, user)
	}

	if err = rows.Err(); err != nil {
		r.logger.WithError(err).Error("An error occurred while reading users")
		return nil, err
	}

	return users, nil
}

func (r *userPostgresRepository) Create(ctx context.Context, dto domain.CreateUserDTO) (domain.User, error) {
	user := domain.User{
		Username:   dto.Username,
		Password:   dto.Password,
		Email:      dto.Email,
		FirstName:  dto.FirstName,
		LastName:   dto.LastName,
		BirthDate:  dto.BirthDate,
		Department: dto.Department,
	}
	query := `
		INSERT INTO users (username, password, first_name, last_name, email, birth_date, department) 
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at
	`

	var birthDate pgtype.Date
	if dto.BirthDate == "" {
		birthDate = pgtype.Date{Status: pgtype.Null}
	} else {
		t, err := time.Parse("2006-01-02", dto.BirthDate)
		if err != nil {
			r.logger.WithError(err).Error("failed to parse birth date %s", dto.BirthDate)
			return domain.User{}, err
		}

		birthDate = pgtype.Date{Time: t, Status: pgtype.Present}
	}

	if err := r.dbPool.QueryRow(
		ctx, query,
		dto.Username, dto.Password, dto.FirstName,
		dto.LastName, dto.Email, birthDate, dto.Department,
	).Scan(&user.ID, &user.CreatedAt); err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			r.logger.WithError(err).Debug("user with such fields is already exist")
			return domain.User{}, domain.ErrUserUniqueViolation
		}

		r.logger.WithError(err).Error("An error occurred while creating user into the database")

		return domain.User{}, err
	}

	return user, nil
}

func (r *userPostgresRepository) GetByID(ctx context.Context, id string) (domain.User, error) {
	if !utils.IsValidUUID(id) {
		r.logger.Debugf("user is not found with id = %s", id)

		return domain.User{}, domain.ErrUserNotFound
	}

	query := `SELECT 
		id, username, password, 
		first_name, last_name, email, 
		birth_date, department, is_deleted, 
		created_at, updated_at
	FROM users WHERE id = $1`

	return r.getBy(ctx, query, id)
}

func (r *userPostgresRepository) GetByUsername(ctx context.Context, username string) (domain.User, error) {
	query := `SELECT 
		id, username, password, 
		first_name, last_name, email, 
		birth_date, department, is_deleted, 
		created_at, updated_at
	FROM users WHERE username = $1 AND is_deleted IS FALSE`

	return r.getBy(ctx, query, username)
}

func (r *userPostgresRepository) getBy(ctx context.Context, query string, args ...interface{}) (domain.User, error) {
	var user domain.User

	var birthDate pgtype.Date

	row := r.dbPool.QueryRow(ctx, query, args...)
	if err := row.Scan(
		&user.ID, &user.Username, &user.Password,
		&user.FirstName, &user.LastName, &user.Email,
		&birthDate, &user.Department, &user.IsDeleted,
		&user.CreatedAt, &user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Debugf("user is not found with these args %v", args)
			return domain.User{}, domain.ErrUserNotFound
		}

		r.logger.WithError(err).Error("An error occurred while getting")

		return domain.User{}, err
	}

	if birthDate.Status == pgtype.Present {
		user.BirthDate = birthDate.Time.Format("2006-01-02")
	}

	return user, nil
}

func (r *userPostgresRepository) Update(ctx context.Context, dto domain.UpdateUserDTO) (domain.User, error) {
	if !utils.IsValidUUID(dto.ID) {
		r.logger.Debugf("user is not found with id = %s", dto.ID)
		return domain.User{}, domain.ErrUserNotFound
	}

	var (
		user      domain.User
		birthDate pgtype.Date
	)

	query := `UPDATE users SET 
		username = $2, email = $3, first_name = $4, 
		last_name = $5, birth_date = $6, department = $7
	WHERE id = $1 AND is_deleted IS FALSE
	RETURNING password, is_deleted, created_at, updated_at`

	if dto.BirthDate != "" {
		tm, err := time.Parse("2006-01-02", dto.BirthDate)
		if err != nil {
			r.logger.WithError(err).Error("")
			return domain.User{}, err
		}

		birthDate = pgtype.Date{
			Time:   tm,
			Status: pgtype.Present,
		}
	} else {
		birthDate = pgtype.Date{Status: pgtype.Null}
	}

	if err := r.dbPool.QueryRow(
		ctx, query, dto.ID,
		dto.Username, dto.Email, dto.FirstName,
		dto.LastName, birthDate, dto.Department,
	).Scan(
		&user.Password, &user.IsDeleted,
		&user.CreatedAt, &user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.logger.Debugf("user is not found with id = %s", dto.ID)
			return domain.User{}, domain.ErrUserNotFound
		}

		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == pgerrcode.UniqueViolation {
			r.logger.WithError(err).Debug("user with such fields is already exist")
			return domain.User{}, domain.ErrUserUniqueViolation
		}

		r.logger.WithError(err).Error("An error occurred while updating user into the database")

		return domain.User{}, err
	}

	user.ID = dto.ID
	user.Username = dto.Username
	user.Email = dto.Email
	user.FirstName = dto.FirstName
	user.LastName = dto.LastName
	user.BirthDate = dto.BirthDate
	user.Department = dto.Department

	return user, nil
}

func (r *userPostgresRepository) UpdatePassword(ctx context.Context, id, password string) error {
	if !utils.IsValidUUID(id) {
		r.logger.Debugf("user is not found with id = %s", id)
		return domain.ErrUserNotFound
	}

	query := "UPDATE users SET password = $2 WHERE id = $1 AND is_deleted IS FALSE"

	cmdTag, err := r.dbPool.Exec(ctx, query, id, password)
	if err != nil {
		r.logger.WithError(err).Error("An error occurred while updating user password into the database")
		return err
	}

	if cmdTag.RowsAffected() == 0 {
		r.logger.Debugf("user is not found with id = %s", id)
		return domain.ErrUserNotFound
	}

	return nil
}

func (r *userPostgresRepository) Delete(ctx context.Context, id string) error {
	if !utils.IsValidUUID(id) {
		r.logger.Debugf("user is not found with id = %s", id)
		return domain.ErrUserNotFound
	}

	query := "UPDATE users SET is_deleted = TRUE WHERE id = $1 AND is_deleted IS FALSE"

	cmgTag, err := r.dbPool.Exec(ctx, query, id)
	if err != nil {
		r.logger.WithError(err).Error("An error occurred while updating user into the database")
		return err
	}

	if cmgTag.RowsAffected() == 0 {
		r.logger.Debugf("user is not found with id = %s", id)
		return domain.ErrUserNotFound
	}

	return nil
}
