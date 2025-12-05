package service

import (
	"log"
	"otterchatbot/internal/core"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"strings"
	"time"
)

type MatchmakerService struct {
	UserRepo *repository.UserRepository
	Bot      *telegram.Client
	I18n     *i18n.I18nService
}

func NewMatchmakerService(repo *repository.UserRepository, bot *telegram.Client, i18n *i18n.I18nService) *MatchmakerService {
	return &MatchmakerService{
		UserRepo: repo,
		Bot:      bot,
		I18n:     i18n,
	}
}

func (s *MatchmakerService) Start() {
	log.Println("Matchmaker service started...")
	
	for {
		s.processMood("dating", true)
		s.processMood("deeptalk", false)
		s.processMood("fun", false)
		s.processMood("debate", false)
		
		time.Sleep(3 * time.Second)
	}
}

func (s *MatchmakerService) processMood(mood string, isStrict bool) {
	users, err := s.UserRepo.GetQueueByMood(mood)
	if err != nil {
		log.Printf("Error fetching queue for %s: %v", mood, err)
		return
	}

	if len(users) < 2 {
		return
	}

	matchedIndices := make(map[int]bool)

	for i := 0; i < len(users); i++ {
		if matchedIndices[i] {
			continue
		}

		for j := i + 1; j < len(users); j++ {
			if matchedIndices[j] {
				continue
			}

			userA := &users[i]
			userB := &users[j]

			if userA.TelegramID == userB.TelegramID {
				continue
			}

			isMatch := s.checkLocationMatch(userA, userB)
			if !isMatch {
				continue
			}

			if isStrict {
				isMatch = s.checkStrictMatch(userA, userB)
			}

			if isMatch {
				s.executeMatch(userA, userB)
				matchedIndices[i] = true
				matchedIndices[j] = true
				break 
			}
		}
	}
}

func (s *MatchmakerService) checkLocationMatch(a, b *core.User) bool {
	locA := a.Location
	locB := b.Location

	// Jika lokasi kosong atau "-", anggap Global (bisa match dengan siapa saja)
	if locA == "" || locA == "-" || locB == "" || locB == "-" {
		return true
	}

	// Jika salah satu memilih "International" (Global), maka match diperbolehkan
	if strings.Contains(locA, "International") || strings.Contains(locB, "International") {
		return true
	}

	// Jika tidak Global, lokasi HARUS SAMA PERSIS
	// Contoh: "🇮🇩 Indonesia" == "🇮🇩 Indonesia" -> True
	// Contoh: "🇮🇩 Indonesia" == "🇲🇾 Malaysia" -> False
	return locA == locB
}

func (s *MatchmakerService) checkStrictMatch(a, b *core.User) bool {
	matchAtoB := (a.Preference == "both" || a.Preference == b.Gender)
	matchBtoA := (b.Preference == "both" || b.Preference == a.Gender)

	return matchAtoB && matchBtoA
}

func (s *MatchmakerService) executeMatch(a, b *core.User) {
	log.Printf("MATCH FOUND: %s (%s) <-> %s (%s)", a.FirstName, a.Location, b.FirstName, b.Location)

	a.Status = "chatting"
	a.PartnerID = b.TelegramID
	
	b.Status = "chatting"
	b.PartnerID = a.TelegramID

	if err := s.UserRepo.Update(a); err != nil {
		log.Printf("Failed to update user A: %v", err)
		return
	}
	if err := s.UserRepo.Update(b); err != nil {
		log.Printf("Failed to update user B: %v", err)
		return
	}

	if a.LastMessageID != 0 {
		_ = s.Bot.DeleteMessage(a.TelegramID, a.LastMessageID)
	}
	if b.LastMessageID != 0 {
		_ = s.Bot.DeleteMessage(b.TelegramID, b.LastMessageID)
	}

	msgA := s.I18n.Get(a.LanguageCode, "partner_found")
	s.Bot.SendMessage(a.TelegramID, msgA)

	msgB := s.I18n.Get(b.LanguageCode, "partner_found")
	s.Bot.SendMessage(b.TelegramID, msgB)
}