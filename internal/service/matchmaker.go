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
		s.processMood("mabar", false)
		
		time.Sleep(3 * time.Second)
	}
}

func (s *MatchmakerService) processMood(mood string, isStrictDefault bool) {
	users, err := s.UserRepo.GetQueueByMood(mood)
	if err != nil { return }

	if len(users) < 2 { return }

	matchedIndices := make(map[int]bool)

	for i := 0; i < len(users); i++ {
		if matchedIndices[i] { continue }

		for j := i + 1; j < len(users); j++ {
			if matchedIndices[j] { continue }

			userA := &users[i]
			userB := &users[j]

			if userA.TelegramID == userB.TelegramID { continue }

			// 1. Cek Lokasi
			if !s.checkLocationMatch(userA, userB) { continue }

			// 2. Logic Match
			isMatch := true
			
			// Cek apakah filter harus aktif
			shouldCheckStrictA := isStrictDefault || userA.IsVIP
			shouldCheckStrictB := isStrictDefault || userB.IsVIP

			// LOGIKA VIP:
			// Jika user VIP, kita hormati preferensinya.
			// TAPI, jika preferensinya "both", berarti dia mau sama siapa aja (Fast Match).
			
			matchAtoB := true
			if shouldCheckStrictA {
				// Jika A VIP dan Pref "Both", otomatis True. Jika tidak, cek gender B.
				matchAtoB = (userA.Preference == "both" || userA.Preference == userB.Gender)
			}

			matchBtoA := true
			if shouldCheckStrictB {
				matchBtoA = (userB.Preference == "both" || userB.Preference == userA.Gender)
			}

			isMatch = matchAtoB && matchBtoA

			if isMatch {
				freshA, errA := s.UserRepo.GetByTelegramID(userA.TelegramID)
				freshB, errB := s.UserRepo.GetByTelegramID(userB.TelegramID)

				// [Pembaruan 2] Tambahkan Cek IsBanned
				if errA != nil || freshA == nil || freshA.Status != "queue" || freshA.IsBanned {
					matchedIndices[i] = true 
					break 
				}
				if errB != nil || freshB == nil || freshB.Status != "queue" || freshB.IsBanned {
					matchedIndices[j] = true 
					continue 
				}

				// Jika aman, eksekusi match menggunakan data TERBARU (freshA & freshB)
				s.executeMatch(freshA, freshB)

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

func (s *MatchmakerService) executeMatch(a, b *core.User) {
	log.Printf("MATCH FOUND: %s (%s) <-> %s (%s)", a.FirstName, a.Location, b.FirstName, b.Location)

	// **[BUG BERADA DI SINI]:** Status diubah di database DULU.
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
	s.Bot.SendMessage(a.TelegramID, msgA) // Pesan dikirim TERAKHIR, jika gagal, status sudah chatting

	msgB := s.I18n.Get(b.LanguageCode, "partner_found")
	s.Bot.SendMessage(b.TelegramID, msgB) // Pesan dikirim TERAKHIR, jika gagal, status sudah chatting
}