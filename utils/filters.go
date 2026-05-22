package utils

import "strings"

var stopWords = map[string]bool{
	"и": true, "в": true, "на": true, "с": true, "по": true, "для": true, "от": true, "к": true,
	"у": true, "за": true, "из": true, "до": true, "об": true, "под": true, "над": true,
	"перед": true, "при": true, "про": true, "через": true, "без": true, "не": true, "но": true,
	"а": true, "или": true, "как": true, "что": true, "чтобы": true, "это": true, "то": true,
	"он": true, "она": true, "они": true, "мы": true, "вы": true, "ты": true, "я": true,
	"меня": true, "мне": true, "мой": true, "твой": true, "свой": true, "его": true, "её": true,
	"их": true, "весь": true, "вся": true, "всё": true, "все": true, "который": true,
	"которая": true, "которые": true, "быть": true, "есть": true, "был": true, "была": true,
	"было": true, "были": true, "будут": true, "будет": true, "сказать": true, "говорить": true,
	"мочь": true, "сделать": true, "ещё": true, "уже": true, "очень": true, "так": true,
	"вот": true, "там": true, "тут": true, "где": true, "когда": true, "почему": true,
	"какой": true, "такая": true, "также": true, "только": true, "даже": true, "просто": true,
	"более": true, "менее": true, "сейчас": true, "сегодня": true, "завтра": true, "вчера": true,
	"потом": true, "всегда": true, "никогда": true, "иногда": true, "вообще": true,
	"конечно": true, "пожалуйста": true, "извините": true, "привет": true, "пока": true,
	"здравствуйте": true, "до свидания": true,
	// Английские
	"the": true, "a": true, "an": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "from": true, "by": true, "with": true, "about": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"will": true, "would": true, "could": true, "should": true, "may": true, "might": true,
	"can": true, "shall": true, "has": true, "have": true, "had": true, "do": true,
	"does": true, "did": true, "and": true, "but": true, "or": true, "not": true,
	"no": true, "yes": true, "so": true, "if": true, "then": true, "else": true,
	"when": true, "where": true, "why": true, "how": true, "all": true, "both": true,
	"each": true, "few": true, "more": true, "most": true, "other": true, "some": true,
	"such": true, "only": true, "own": true, "same": true, "than": true, "too": true,
	"very": true, "just": true, "now": true, "here": true, "there": true,
}

func IsKidSafe(title, description string) bool {
	stopWords := []string{
		"убил", "застрелили", "смерть", "погиб", "трагедия", "катастрофа",
		"взорвал", "террорист", "жесток", "насилие", "ограбление", "изнасилование",
		"наркотик", "алкоголь", "взрослый контент", "18+", "эротик", "порно",
		"азартные игры", "казино", "ставки на спорт",
	}
	combined := strings.ToLower(title + " " + description)
	for _, word := range stopWords {
		if strings.Contains(combined, word) {
			return false
		}
	}
	return true
}

func ExtractKeywordsFromTitle(title string) string {
	var clean strings.Builder
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') ||
			(r >= '0' && r <= '9') || r == ' ' {
			clean.WriteRune(r)
		}
	}
	words := strings.Fields(clean.String())
	var meaningful []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) <= 2 || stopWords[lower] {
			continue
		}
		meaningful = append(meaningful, lower)
	}
	if len(meaningful) > 5 {
		meaningful = meaningful[:5]
	}
	return Transliterate(strings.Join(meaningful, " "))
}

func Transliterate(s string) string {
	repl := strings.NewReplacer(
		"а", "a", "б", "b", "в", "v", "г", "g", "д", "d", "е", "e", "ё", "yo",
		"ж", "zh", "з", "z", "и", "i", "й", "y", "к", "k", "л", "l", "м", "m",
		"н", "n", "о", "o", "п", "p", "р", "r", "с", "s", "т", "t", "у", "u",
		"ф", "f", "х", "kh", "ц", "ts", "ч", "ch", "ш", "sh", "щ", "shch",
		"ъ", "", "ы", "y", "ь", "", "э", "e", "ю", "yu", "я", "ya",
		"А", "A", "Б", "B", "В", "V", "Г", "G", "Д", "D", "Е", "E", "Ё", "Yo",
		"Ж", "Zh", "З", "Z", "И", "I", "Й", "Y", "К", "K", "Л", "L", "М", "M",
		"Н", "N", "О", "O", "П", "P", "Р", "R", "С", "S", "Т", "T", "У", "U",
		"Ф", "F", "Х", "Kh", "Ц", "Ts", "Ч", "Ch", "Ш", "Sh", "Щ", "Shch",
		"Ъ", "", "Ы", "Y", "Ь", "", "Э", "E", "Ю", "Yu", "Я", "Ya",
	)
	return repl.Replace(s)
}

func ExtractRussianKeywords(title string) string {
	var clean strings.Builder
	for _, r := range title {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') ||
			(r >= '0' && r <= '9') || r == ' ' {
			clean.WriteRune(r)
		}
	}
	words := strings.Fields(clean.String())
	var meaningful []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if len(lower) <= 2 || stopWords[lower] {
			continue
		}
		meaningful = append(meaningful, lower)
	}
	if len(meaningful) > 5 {
		meaningful = meaningful[:5]
	}
	return strings.Join(meaningful, " ")
}

// extractRussianNouns оставляет слова длиннее 3 букв, удаляя стоп-слова
func ExtractRussianNouns(title string) string {
	stopWords := map[string]bool{
		"это": true, "как": true, "что": true, "для": true, "новый": true,
		"самый": true, "ещё": true, "уже": true, "очень": true, "быть": true,
		"весь": true, "они": true, "она": true, "оно": true, "там": true,
		"где": true, "когда": true, "почему": true, "зачем": true, "или": true,
		"под": true, "над": true, "перед": true, "около": true, "через": true,
	}
	words := strings.Fields(title)
	var clean []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if len([]rune(w)) > 3 && !stopWords[strings.ToLower(w)] {
			clean = append(clean, w)
		}
	}
	if len(clean) == 0 {
		return title
	}
	return strings.Join(clean, " ")
}
