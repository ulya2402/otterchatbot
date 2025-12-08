package service

import (
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"
)

type GameData struct {
	Truth []string `json:"truth"`
	Dare  []string `json:"dare"`
}

// Map key: language code (id, en, ru)
type GameConfig map[string]GameData

type GameService struct {
	Questions GameConfig
	mu        sync.RWMutex
}

func NewGameService() *GameService {
	s := &GameService{
		Questions: make(GameConfig),
	}
	s.loadQuestions()
	return s
}

func (s *GameService) loadQuestions() {
	file, err := os.ReadFile("config/games.json")
	if err != nil {
		log.Printf("Warning: Could not load config/games.json: %v", err)
		return
	}

	if err := json.Unmarshal(file, &s.Questions); err != nil {
		log.Printf("Error parsing games.json: %v", err)
	} else {
		log.Printf("Loaded game questions for %d languages.", len(s.Questions))
	}
}

func (s *GameService) GetQuestion(lang string, category string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default ke bahasa Inggris jika bahasa user tidak ditemukan
	data, ok := s.Questions[lang]
	if !ok {
		data, ok = s.Questions["en"]
		if !ok {
			return "No questions available."
		}
	}

	var list []string
	if category == "truth" {
		list = data.Truth
	} else {
		list = data.Dare
	}

	if len(list) == 0 {
		return "No questions in this category."
	}

	// Acak pertanyaan
	rand.New(rand.NewSource(time.Now().UnixNano()))
	index := rand.Intn(len(list))
	return list[index]
}