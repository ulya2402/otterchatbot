package config

import (
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
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found, relying on system environment variables")
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

	if cfg.BotToken == "" {
		log.Fatal("Fatal: BOT_TOKEN is required")
	}
	if cfg.SupabaseURL == "" {
		log.Fatal("Fatal: SUPABASE_URL is required")
	}
	if cfg.SupabaseKey == "" {
		log.Fatal("Fatal: SUPABASE_KEY is required")
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}