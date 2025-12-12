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

	gameService := service.NewGameService()

	userRepo := repository.NewUserRepository(supabaseClient)
	botClient := telegram.NewClient(cfg.BotToken)
	botHandler := handler.NewBotHandler(botClient, userRepo, translator, cfg, gameService)
	matchmakerService := service.NewMatchmakerService(userRepo, botClient, translator)

	log.Println("Registering bot commands to Telegram...")
	registerCommands(botClient)

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

func registerCommands(bot *telegram.Client) {
	// 1. DEFAULT (Inggris)
	cmdsEn := []telegram.BotCommand{
		{Command: "start", Description: "👋 Main Menu / Restart"},
		{Command: "search", Description: "🔍 Find a partner"},
		{Command: "next", Description: "⏭ Skip & search new"},
		{Command: "stop", Description: "⛔ End chat"},
		{Command: "profile", Description: "👤 My Profile"},
		{Command: "report", Description: "🚨 Report User"},
		{Command: "vip", Description: "🌟 VIP Upgrade"},
		{Command: "help", Description: "❓ Help Center"},
		{Command: "lang", Description: "🌐 Change Language"}, // <--- SUDAH DITAMBAHKAN
	}
	_ = bot.SetMyCommands(cmdsEn, "")   // Global
	_ = bot.SetMyCommands(cmdsEn, "en") // English users

	// 2. INDONESIA
	cmdsId := []telegram.BotCommand{
		{Command: "start", Description: "👋 Menu Utama"},
		{Command: "search", Description: "🔍 Cari teman"},
		{Command: "next", Description: "⏭ Ganti partner"},
		{Command: "stop", Description: "⛔ Akhiri chat"},
		{Command: "profile", Description: "👤 Profil Saya"},
		{Command: "report", Description: "🚨 Lapor Toxic"},
		{Command: "vip", Description: "🌟 Beli VIP"},
		{Command: "help", Description: "❓ Bantuan"},
		{Command: "lang", Description: "🌐 Ganti Bahasa"}, // <--- SUDAH DITAMBAHKAN
	}
	_ = bot.SetMyCommands(cmdsId, "id")

	// 3. RUSSIA
	cmdsRu := []telegram.BotCommand{
		{Command: "start", Description: "👋 Главное меню"},
		{Command: "search", Description: "🔍 Найти"},
		{Command: "next", Description: "⏭ Следующий"},
		{Command: "stop", Description: "⛔ Стоп"},
		{Command: "profile", Description: "👤 Профиль"},
		{Command: "report", Description: "🚨 Жалоба"},
		{Command: "vip", Description: "🌟 VIP"},
		{Command: "help", Description: "❓ Помощь"},
		{Command: "lang", Description: "🌐 Сменить язык"}, // <--- SUDAH DITAMBAHKAN
	}
	_ = bot.SetMyCommands(cmdsRu, "ru")
}