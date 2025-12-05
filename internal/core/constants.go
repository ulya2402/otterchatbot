package core

type Option struct {
	Code  string
	Label string 
	Icon  string 
}

var AvailableLanguages = []Option{
	{Code: "id", Label: "Indonesia", Icon: "🇮🇩"},
	{Code: "en", Label: "English", Icon: "🇺🇸"},
	{Code: "ru", Label: "Русский", Icon: "🇷🇺"},
}

var AvailableCountries = []Option{
	{Code: "ID", Label: "Indonesia", Icon: "🇮🇩"},
	{Code: "MY", Label: "Malaysia", Icon: "🇲🇾"},
	{Code: "SG", Label: "Singapore", Icon: "🇸🇬"},
	{Code: "RU", Label: "Russia", Icon: "🇷🇺"},
	{Code: "US", Label: "USA", Icon: "🇺🇸"},
	{Code: "IN", Label: "India", Icon: "🇮🇳"},
	{Code: "GLOBAL", Label: "International", Icon: "🌍"},
}

var AvailableMoods = []Option{
	{Code: "dating", Label: "mood_dating", Icon: ""},
	{Code: "deeptalk", Label: "mood_deeptalk", Icon: ""},
	{Code: "fun", Label: "mood_fun", Icon: ""},
	{Code: "debate", Label: "mood_debate", Icon: ""},
	{Code: "mabar", Label: "mood_mabar", Icon: ""},
}