package service

import (
	"log"
	"otterchatbot/internal/core"
	"otterchatbot/internal/repository"
	"otterchatbot/pkg/i18n"
	"otterchatbot/pkg/telegram"
	"strings"
	"fmt"
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
		s.processMood("all", false)
		
		time.Sleep(3 * time.Second)
	}
}

func mergeUsers(specific []core.User, general []core.User) []core.User {
	// Gunakan map untuk mencegah duplikat (jika ada logic error di db)
	seen := make(map[int64]bool)
	var result []core.User

	// Masukkan user spesifik (misal: dating)
	for _, u := range specific {
		if !seen[u.TelegramID] {
			seen[u.TelegramID] = true
			result = append(result, u)
		}
	}

	// Masukkan user general (all)
	for _, u := range general {
		if !seen[u.TelegramID] {
			seen[u.TelegramID] = true
			result = append(result, u)
		}
	}
	return result
}

func (s *MatchmakerService) processMood(mood string, isStrictDefault bool) {
	// 1. Ambil user yang MEMANG milih mood ini
	specificUsers, err := s.UserRepo.GetQueueByMood(mood)
	if err != nil { return }

	// 2. Ambil user yang milih "ALL" (Fast Match)
	// Kecuali jika kita memang sedang memproses mood "all", tidak perlu fetch ulang
	var poolUsers []core.User
	
	if mood == "all" {
		poolUsers = specificUsers
	} else {
		allUsers, err := s.UserRepo.GetQueueByMood("all")
		if err == nil {
			// Gabungkan: User Mood Ini + User Mood 'All'
			poolUsers = mergeUsers(specificUsers, allUsers)
		} else {
			poolUsers = specificUsers
		}
	}

	if len(poolUsers) < 2 { return }

	matchedIndices := make(map[int64]bool) // Ubah key jadi TelegramID biar unik

	for i := 0; i < len(poolUsers); i++ {
		userA := &poolUsers[i]
		if matchedIndices[userA.TelegramID] { continue }

		for j := i + 1; j < len(poolUsers); j++ {
			userB := &poolUsers[j]
			if matchedIndices[userB.TelegramID] { continue }

			if userA.TelegramID == userB.TelegramID { continue }

			// 1. Cek Lokasi
			if !s.checkLocationMatch(userA, userB) { continue }

			// 2. Logic Match
			isMatch := true
			
			// Cek apakah filter harus aktif
			shouldCheckStrictA := isStrictDefault || userA.IsVIP
			shouldCheckStrictB := isStrictDefault || userB.IsVIP

			// LOGIKA VIP & PREFERENSI:
			// Preferensi user tersimpan di Profil Global.
			// Jadi meskipun user "All" masuk ke pool "Dating", filter gendernya tetap aktif.
			
			matchAtoB := true
			if shouldCheckStrictA {
				matchAtoB = (userA.Preference == "both" || userA.Preference == userB.Gender)
			}

			matchBtoA := true
			if shouldCheckStrictB {
				matchBtoA = (userB.Preference == "both" || userB.Preference == userA.Gender)
			}

			isMatch = matchAtoB && matchBtoA

			if isMatch {
				// [Validasi Akhir] Pastikan status DB masih queue (Anti Race Condition sederhana)
				freshA, errA := s.UserRepo.GetByTelegramID(userA.TelegramID)
				freshB, errB := s.UserRepo.GetByTelegramID(userB.TelegramID)

				if errA != nil || freshA == nil || freshA.Status != "queue" || freshA.IsBanned {
					matchedIndices[userA.TelegramID] = true 
					break 
				}
				if errB != nil || freshB == nil || freshB.Status != "queue" || freshB.IsBanned {
					matchedIndices[userB.TelegramID] = true 
					continue 
				}

				// Eksekusi Match
				s.executeMatch(freshA, freshB, mood) // Kirim mood biar user tau ketemu di topik apa

				matchedIndices[userA.TelegramID] = true
				matchedIndices[userB.TelegramID] = true
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

func (s *MatchmakerService) buildMatchCard(receiver *core.User, partner *core.User, topic string) string {
	title := s.I18n.Get(receiver.LanguageCode, "match_title")
	lblTopic := s.I18n.Get(receiver.LanguageCode, "match_topic")
	lblTip := s.I18n.Get(receiver.LanguageCode, "match_tip")

	locText := partner.Location
	if locText == "" || locText == "-" {
		locText = "Global 🌍"
	}

	msg := fmt.Sprintf("%s\n\n", title)
	msg += fmt.Sprintf("🎭 %s: <code>%s</code>\n\n", lblTopic, strings.ToUpper(topic))

	msg += fmt.Sprintf("%s", lblTip)

	return msg
}

func (s *MatchmakerService) executeMatch(a, b *core.User, topic string) {
	log.Printf("MATCH FOUND (%s): %s <-> %s", topic, a.FirstName, b.FirstName)

	a.Status = "chatting"
	a.PartnerID = b.TelegramID
	
	b.Status = "chatting"
	b.PartnerID = a.TelegramID

	if err := s.UserRepo.Update(a); err != nil { return }
	if err := s.UserRepo.Update(b); err != nil { return }

	if a.LastMessageID != 0 { _ = s.Bot.DeleteMessage(a.TelegramID, a.LastMessageID) }
	if b.LastMessageID != 0 { _ = s.Bot.DeleteMessage(b.TelegramID, b.LastMessageID) }

	// Format pesan notifikasi
	// Kita bisa modifikasi text locale nanti, misal: "Partner Found! (Topic: Dating)"
	// Untuk sekarang pakai default dulu
	
	msgA := s.buildMatchCard(a, b, topic)
	s.Bot.SendMessageComplex(telegram.SendMessageRequest{
		ChatID: a.TelegramID,
		Text: msgA,
		ParseMode: "HTML",
	})

	// --- KIRIM MATCH CARD KE USER B ---
	// User B melihat Data User A
	msgB := s.buildMatchCard(b, a, topic)
	s.Bot.SendMessageComplex(telegram.SendMessageRequest{
		ChatID: b.TelegramID,
		Text: msgB,
		ParseMode: "HTML",
	})
}

