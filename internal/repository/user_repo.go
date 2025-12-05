package repository

import (
	"fmt"
	"log"
	"otterchatbot/internal/core"
	"otterchatbot/pkg/database"
	"time"
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

	user := &users[0]

	// --- LOGIKA OTOMATIS: Cek Expired VIP ---
	if r.checkVipExpiration(user) {
		// Jika expired, update database sekarang juga
		_ = r.Update(user)
		log.Printf("User %d VIP expired and has been downgraded.", user.TelegramID)
	}

	return user, nil
}

// Fungsi internal untuk mengecek & downgrade VIP
func (r *UserRepository) checkVipExpiration(user *core.User) bool {
	// 1. Jika bukan VIP, abaikan
	if !user.IsVIP {
		return false
	}

	// 2. Jika tanggal expired kosong (VIP Seumur Hidup), abaikan
	if user.VipExpiresAt == nil {
		return false
	}

	// 3. Cek apakah Waktu Sekarang > Waktu Expired
	if time.Now().After(*user.VipExpiresAt) {
		user.IsVIP = false       // Cabut status VIP
		user.VipExpiresAt = nil  // Hapus tanggalnya
		return true              // Beri tahu bahwa ada perubahan data
	}

	return false
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

func (r *UserRepository) CountAll() (int64, error) {
	// FIX: Hapus var count int64 yang tidak terpakai
	var results []struct{ ID int64 }
	err := r.DB.Client.DB.From("users").Select("id", "exact").Execute(&results)
	if err != nil {
		return 0, err
	}
	return int64(len(results)), nil
}

// GetLiveStats mengambil data real-time
func (r *UserRepository) GetLiveStats() (int, int, int) {
	var chatting []core.User
	var queue []core.User
	var vip []core.User

	_ = r.DB.Client.DB.From("users").Select("*").Eq("status", "chatting").Execute(&chatting)
	_ = r.DB.Client.DB.From("users").Select("*").Eq("status", "queue").Execute(&queue)
	_ = r.DB.Client.DB.From("users").Select("*").Eq("is_vip", "true").Execute(&vip)

	return len(chatting), len(queue), len(vip)
}

// GetAllTelegramIDs mengambil semua ID user untuk broadcast (Hati-hati, query berat jika user jutaan)
func (r *UserRepository) GetAllTelegramIDs() ([]int64, error) {
	var users []core.User
	// Ambil telegram_id saja
	err := r.DB.Client.DB.From("users").Select("telegram_id").Execute(&users)
	if err != nil {
		return nil, err
	}

	var ids []int64
	for _, u := range users {
		ids = append(ids, u.TelegramID)
	}
	return ids, nil
}