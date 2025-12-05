package main

import (
	"log"
	"otterchatbot/config"
	"otterchatbot/internal/handler"
	"otterchatbot/internal/repository"
	"otterchatbot/internal/service"
	"otterchatbot/pkg/database"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"time"
)

func main() {
	log.Println("Starting OtterChatbot system...")

	cfg := config.LoadConfig()

	translator := i18n.NewI18n(cfg.DefaultLang)
	if err := translator.LoadLanguages("./locales"); err != nil {
		log.Fatalf("Fatal: Failed to load locales: %v", err)
	}

	supabaseClient, err := database.Connect(cfg.SupabaseURL, cfg.SupabaseKey)
	if err != nil {
		log.Fatalf("Fatal: Could not initialize Supabase client: %v", err)
	}

	userRepo := repository.NewUserRepository(supabaseClient)
	botClient := telegram.NewClient(cfg.BotToken)
	botHandler := handler.NewBotHandler(botClient, userRepo, translator, cfg)
	matchmakerService := service.NewMatchmakerService(userRepo, botClient, translator)

	// Jalankan Matchmaker di background (Goroutine)
	go matchmakerService.Start()

	log.Println("Bot is running. Polling for updates...")
	
	offset := 0
	for {
		updates, err := botClient.GetUpdates(offset)
		if err != nil {
			log.Printf("Error fetching updates: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}

			go botHandler.HandleUpdate(update)
		}
		
		time.Sleep(500 * time.Millisecond)
	}
}