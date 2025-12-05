package config

import (
	"encoding/json"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv      string
	BotToken    string
	SupabaseURL string
	SupabaseKey string
	AdminIDs    []string
	DefaultLang string
	// [BARU] Menyimpan daftar paket VIP
	VIPPlans    []VIPPlan
}

// [BARU] Struktur data untuk paket VIP
type VIPPlan struct {
	ID       string `json:"id"`
	Days     int    `json:"days"`
	Price    int    `json:"price"`
	TitleKey string `json:"title_key"`
	DescKey  string `json:"desc_key"`
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found")
	}

	cfg := &Config{
		AppEnv:      getEnv("APP_ENV", "development"),
		BotToken:    getEnv("BOT_TOKEN", ""),
		SupabaseURL: getEnv("SUPABASE_URL", ""),
		SupabaseKey: getEnv("SUPABASE_KEY", ""),
		DefaultLang: getEnv("DEFAULT_LANG", "en"),
	}

	admins := getEnv("ADMIN_IDS", "")
	if admins != "" {
		cfg.AdminIDs = strings.Split(admins, ",")
	}

	if cfg.BotToken == "" { log.Fatal("Fatal: BOT_TOKEN required") }
	if cfg.SupabaseURL == "" { log.Fatal("Fatal: SUPABASE_URL required") }

	// [BARU] Load Pricing JSON
	cfg.loadPricing()

	return cfg
}

func (c *Config) loadPricing() {
	file, err := os.ReadFile("config/pricing.json")
	if err != nil {
		log.Printf("Warning: Could not load config/pricing.json: %v. VIP features might fail.", err)
		return
	}
	
	if err := json.Unmarshal(file, &c.VIPPlans); err != nil {
		log.Printf("Error parsing pricing.json: %v", err)
	} else {
		log.Printf("Loaded %d VIP plans from config.", len(c.VIPPlans))
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}