package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type I18nService struct {
	translations map[string]map[string]string
	mu           sync.RWMutex
	defaultLang  string
}

func NewI18n(defaultLang string) *I18nService {
	return &I18nService{
		translations: make(map[string]map[string]string),
		defaultLang:  defaultLang,
	}
}

func (s *I18nService) LoadLanguages(localesDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	files, err := os.ReadDir(localesDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		langCode := file.Name()[0 : len(file.Name())-5]
		filePath := filepath.Join(localesDir, file.Name())

		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read locale file %s: %v", filePath, err)
		}

		var data map[string]string
		if err := json.Unmarshal(content, &data); err != nil {
			return fmt.Errorf("failed to parse json %s: %v", filePath, err)
		}

		s.translations[langCode] = data
	}

	return nil
}

func (s *I18nService) Get(lang, key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if langData, ok := s.translations[lang]; ok {
		if val, ok := langData[key]; ok {
			return val
		}
	}

	if defaultData, ok := s.translations[s.defaultLang]; ok {
		if val, ok := defaultData[key]; ok {
			return val
		}
	}

	return key
}