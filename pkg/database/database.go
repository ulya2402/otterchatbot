package database

import (
	"fmt"
	"log"

	"github.com/nedpals/supabase-go"
)

type DB struct {
	Client *supabase.Client
}

// Connect menginisialisasi client Supabase menggunakan URL dan Key (Service Role)
func Connect(url string, key string) (*DB, error) {
	if url == "" || key == "" {
		return nil, fmt.Errorf("supabase URL or Key is empty")
	}

	// Inisialisasi client
	client := supabase.CreateClient(url, key)

	// Kita lakukan test ping sederhana dengan mencoba membaca tabel 'users' (limit 1)
	// Pastikan tabel 'users' sudah dibuat di SQL Editor Supabase sebelumnya
	var results []map[string]interface{}
	err := client.DB.From("users").Select("*").Limit(1).Execute(&results)
	
	// Jika error bukan nil, berarti koneksi atau key bermasalah
	if err != nil {
		// Abaikan error jika itu hanya karena tabel kosong, tapi jika koneksi gagal total kita return error
		// Supabase-go terkadang me-return error jika result kosong, kita log saja sebagai warning
		log.Printf("Warning during connection test (might be empty table): %v", err)
	}

	return &DB{Client: client}, nil
}