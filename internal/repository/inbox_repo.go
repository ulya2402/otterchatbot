package repository

import (
	"fmt"
	"log"
	"otterchatbot/internal/core"
	"otterchatbot/pkg/database"
	"sort" 
)

type InboxRepository struct {
	DB *database.DB
}

func NewInboxRepository(db *database.DB) *InboxRepository {
	return &InboxRepository{DB: db}
}

func (r *InboxRepository) SaveMessage(msg *core.InboxMessage) error {
	var results []core.InboxMessage
	err := r.DB.Client.DB.From("inbox_messages").Insert(msg).Execute(&results)
	if err != nil {
		log.Printf("Failed to insert inbox message: %v", err)
		return err
	}
	return nil
}

func (r *InboxRepository) GetMessagesByReceiver(receiverID int64) ([]core.InboxMessage, error) {
	var messages []core.InboxMessage
	idStr := fmt.Sprintf("%d", receiverID)
	
	// PEMBARUAN: Hapus .Order() dari sini untuk menghindari error library
	err := r.DB.Client.DB.From("inbox_messages").
		Select("*").
		Eq("receiver_id", idStr).
		Execute(&messages)

	if err != nil {
		return nil, err
	}

	// PEMBARUAN: Lakukan sorting manual di Go (Lebih aman)
	// Urutkan dari yang Terlama (index 0) ke Terbaru
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	return messages, nil
}

func (r *InboxRepository) DeleteMessagesByReceiver(receiverID int64) error {
	var results []core.InboxMessage
	idStr := fmt.Sprintf("%d", receiverID)
	
	err := r.DB.Client.DB.From("inbox_messages").
		Delete().
		Eq("receiver_id", idStr).
		Execute(&results)
		
	return err
}

// Tambahkan fungsi ini di bagian paling bawah file inbox_repo.go

func (r *InboxRepository) GetMessageByID(id int64) (*core.InboxMessage, error) {
	var messages []core.InboxMessage
	idStr := fmt.Sprintf("%d", id)
	
	err := r.DB.Client.DB.From("inbox_messages").
		Select("*").
		Eq("id", idStr).
		Execute(&messages)

	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	return &messages[0], nil
}