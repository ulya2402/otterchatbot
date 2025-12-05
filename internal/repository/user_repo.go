package repository

import (
	"fmt"
	"log"
	"otterchatbot/internal/core"
	"otterchatbot/pkg/database"
)

type UserRepository struct {
	DB *database.DB
}

func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{DB: db}
}

func (r *UserRepository) GetByTelegramID(telegramID int64) (*core.User, error) {
	var users []core.User
	idStr := fmt.Sprintf("%d", telegramID)
	err := r.DB.Client.DB.From("users").Select("*").Eq("telegram_id", idStr).Execute(&users)
	if err != nil {
		return nil, fmt.Errorf("error fetching user: %v", err)
	}
	if len(users) == 0 {
		return nil, nil 
	}
	return &users[0], nil
}

func (r *UserRepository) Create(user *core.User) error {
	var results []core.User
	err := r.DB.Client.DB.From("users").Insert(user).Execute(&results)
	if err != nil {
		log.Printf("Failed to insert user: %v", err)
		return err
	}
	return nil
}

func (r *UserRepository) Update(user *core.User) error {
	var results []core.User
	idStr := fmt.Sprintf("%d", user.TelegramID)
	// Pastikan field baru ikut terupdate
	err := r.DB.Client.DB.From("users").Update(user).Eq("telegram_id", idStr).Execute(&results)
	if err != nil {
		log.Printf("Failed to update user: %v", err)
		return err
	}
	return nil
}

func (r *UserRepository) GetQueueByMood(mood string) ([]core.User, error) {
	var users []core.User
	err := r.DB.Client.DB.From("users").
		Select("*").
		Eq("status", "queue").
		Eq("current_mood", mood).
		Execute(&users)
	if err != nil {
		return nil, err
	}
	return users, nil
}