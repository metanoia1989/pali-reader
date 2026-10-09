package importer

import (
	"strings"
	"unicode/utf8"
)

// Chinese titles for the canon.
//
// The upstream books table carries Pāḷi titles only. Rather than hand-write
// 179 strings — which would rot the moment upstream renames a volume — the
// titles are composed from a term dictionary plus the volume suffix the
// source already uses. A term that is not in the dictionary leaves the Pāḷi
// word in place, which is honest: a scholar reads the Pāḷi title anyway, and
// a wrong translation is worse than an untranslated one.

// termZh holds the vocabulary the titles are built from. Keys are lower-case
// Pāḷi, matching the source spelling exactly.
var termZh = map[string]string{
	// piṭaka and nikāya
	"vinayapiṭaka":     "律藏",
	"suttantapiṭaka":   "经藏",
	"abhidhammapiṭaka": "论藏",
	"dīghanikāya":      "长部",
	"majjhimanikāya":   "中部",
	"saṃyuttanikāya":   "相应部",
	"aṅguttaranikāya":  "增支部",
	"khuddakanikāya":   "小部",
	"pāḷi":             "",

	// Vinaya
	"pārājika":  "波罗夷",
	"pācittiya": "波逸提",
	"mahāvagga": "大品",
	"cūḷavagga": "小品",
	"parivāra":  "附随",
	"kaṇḍa":     "篇",

	// Dīgha / Majjhima
	"sīlakkhandhavagga": "戒蕴品",
	"mahāvagga_di":      "大品",
	"pāthikavagga":      "波梨品",
	"mūlapaṇṇāsa":       "根本五十经",
	"majjhimapaṇṇāsa":   "中分五十经",
	"uparipaṇṇāsa":      "后分五十经",

	// Saṃyutta
	"sagāthāvagga":      "有偈品",
	"nidānavagga":       "因缘品",
	"khandhavagga":      "蕴品",
	"saḷāyatanavagga":   "六处品",
	"mahāvaggasaṃyutta": "大品相应",

	// Aṅguttara
	"ekakanipāta":     "一集",
	"dukanipāta":      "二集",
	"dukādinipāta":    "二集等",
	"tikanipāta":      "三集",
	"catukkanipāta":   "四集",
	"pañcakanipāta":   "五集",
	"pañcakādinipāta": "五集等",
	"chakkanipāta":    "六集",
	"sattakanipāta":   "七集",
	"aṭṭhakanipāta":   "八集",
	"aṭṭhakādinipāta": "八集等",
	"navakanipāta":    "九集",
	"dasakanipāta":    "十集",
	"ekādasakanipāta": "十一集",

	// Abhidhamma
	"dhammasaṅgaṇī":   "法聚论",
	"vibhaṅga":        "分别论",
	"dhātukathā":      "界论",
	"puggalapaññatti": "人施设论",
	"kathāvatthu":     "论事",
	"yamaka":          "双论",
	"paṭṭhāna":        "发趣论",
	"dhātukathāpāḷi":  "界论",

	// Khuddaka
	"khuddakapāṭha":     "小诵",
	"dhammapada":        "法句",
	"udāna":             "自说",
	"itivuttaka":        "如是语",
	"suttanipāta":       "经集",
	"vimānavatthu":      "天宫事",
	"petavatthu":        "饿鬼事",
	"theragāthā":        "长老偈",
	"therīgāthā":        "长老尼偈",
	"apadāna":           "譬喻",
	"buddhavaṃsa":       "佛种姓",
	"cariyāpiṭaka":      "所行藏",
	"jātaka":            "本生",
	"mahāniddesa":       "大义释",
	"cūḷaniddesa":       "小义释",
	"paṭisambhidāmagga": "无碍解道",
	"milindapañha":      "弥兰王问",
	"nettippakaraṇa":    "导论",
	"peṭakopadesa":      "藏释",

	// Commentaries and sub-commentaries
	"aṭṭhakathā":              "义注",
	"ṭīkā":                    "复注",
	"mūlaṭīkā":                "根本复注",
	"anuṭīkā":                 "随复注",
	"abhinavaṭīkā":            "新复注",
	"mahāṭīkā":                "大复注",
	"visuddhimagga":           "清净道论",
	"abhidhammatthasaṅgaha":   "摄阿毗达摩义论",
	"abhidhammatthavibhāvinī": "摄阿毗达摩义显",
	"paramatthadīpanī":        "胜义显",
	"vinayavinicchaya":        "律决定",
	"uttaravinicchaya":        "后决定",
	"kaṅkhāvitaraṇī":          "疑解",
	"sāratthadīpanī":          "义显",
	"vimativinodanī":          "除疑",
	"vajirabuddhi":            "金刚觉",
	"vinayālaṅkāra":           "律庄严",
	"vinayasaṅgaha":           "律摄",
	"khuddasikkhā":            "小戒",
	"mūlasikkhā":              "根本戒",
	"dvemātikā":               "双母论",
	"pācityādiyojanā":         "波逸提等释",
	"nettivibhāvinī":          "导论显",
	"abhidhānappadīpikā":      "名词灯",
	"subodhālaṅkāra":          "庄严明解",
	"vuttodaya":               "韵律",
	"padarūpasiddhi":          "词形成就",
	"niruttidīpanī":           "词源显",
	"payogasiddhi":            "应用成就",
	"kaccāyana":               "迦旃延文法",
	"kaccāyanasāra":           "迦旃延文法精要",
	"saddanīti":               "善语法",
	"moggallāna":              "目犍连文法",
	"saddatthabhedacintā":     "善义差别思",
	"saṃyutta":                "相应",
	"byākaraṇa":               "文法",
	"suttapāṭha":              "经诵",
	"pakaraṇa":                "论书",
	"pāṭha":                   "诵本",
	"abhinava":                "新",
	"mātikā":                  "母论",
	"anudīpanī":               "随显",
	"vibhāvinī":               "显",
	"pañcikā":                 "注",
	"purāṇa":                  "古",
	"kaṅkhā":                  "疑",
	"pañcapakaraṇa":           "五论",
}

// categoryZh names the divisions the source groups books into. The source
// writes them as running Pāḷi ("suttantapiṭaka (dīghanikāya)"), which is precise
// but not what a Chinese reader scans for.
var categoryZh = map[string]string{
	"vi":          "律藏",
	"di":          "经藏 · 长部",
	"ma":          "经藏 · 中部",
	"sa":          "经藏 · 相应部",
	"an":          "经藏 · 增支部",
	"ku":          "经藏 · 小部",
	"bi":          "论藏",
	"annya_vi":    "藏外 · 律",
	"annya_bi":    "藏外 · 论",
	"annya_sadda": "藏外 · 文法",
}

// categoryPi is the same division as the Pāḷi a reader would cite. The source
// writes "suttantapiṭaka (dīghanikāya)", which is precise about where the file
// sits and useless in a rail; the division itself is called Dīghanikāya.
var categoryPi = map[string]string{
	"vi":          "Vinayapiṭaka",
	"di":          "Dīghanikāya",
	"ma":          "Majjhimanikāya",
	"sa":          "Saṃyuttanikāya",
	"an":          "Aṅguttaranikāya",
	"ku":          "Khuddakanikāya",
	"bi":          "Abhidhammapiṭaka",
	"annya_vi":    "Pakiṇṇaka · Vinaya",
	"annya_bi":    "Pakiṇṇaka · Abhidhamma",
	"annya_sadda": "Pakiṇṇaka · Byākaraṇa",
}

// CategoryNamePi returns the Pāḷi name for a division, falling back to the
// source's own name.
func CategoryNamePi(id, paliName string) string {
	if pi, ok := categoryPi[id]; ok {
		return pi
	}
	return paliName
}

// CategoryNameZh returns the Chinese name for a division, falling back to the
// source's own name.
func CategoryNameZh(id, paliName string) string {
	if zh, ok := categoryZh[id]; ok {
		return zh
	}
	return paliName
}

// volumeZh renders the "(pa)" / "(du)" suffixes the source uses to separate
// volumes of one work.
var volumeZh = map[string]string{
	"pa":    "第一册",
	"du":    "第二册",
	"ta":    "第三册",
	"ca":    "第四册",
	"pañca": "第五册",
	"cha":   "第六册",
	"satta": "第七册",
	// The three parts of the Saddanīti, which the source writes in the same
	// bracket that it uses for volumes.
	"padamālā":  "词鬘",
	"dhātumālā": "界鬘",
	"suttamālā": "经鬘",
}

// ChineseName composes a Chinese title for a book from its Pāḷi name.
//
// The source writes titles as running Pāḷi, so two spelling changes have to be
// undone before the term dictionary will match:
//
//   - sandhi elision: "sīlakkhandhavagga" + "aṭṭhakathā" is written
//     "sīlakkhandhavaggaṭṭhakathā" with a single a. Matching terms literally
//     left half the title untranslated ("戒蕴品ṭṭhakathā").
//   - stem endings: the source writes "visuddhimaggo", the dictionary knows
//     "visuddhimagga". The nominative ending has to be allowed for.
//
// Getting this wrong is cosmetic rather than dangerous — an unmatched run falls
// through as the Pāḷi it already was — so the matching is allowed to be
// generous.
func ChineseName(paliName string) string {
	name := strings.TrimSpace(paliName)
	volume := ""
	if i := strings.LastIndex(name, "("); i > 0 && strings.HasSuffix(name, ")") {
		key := strings.TrimSpace(name[i+1 : len(name)-1])
		if zh, ok := volumeZh[key]; ok {
			volume = zh
			name = strings.TrimSpace(name[:i])
		}
	}

	lower := strings.ToLower(name)
	rest := lower
	prevEndedInVowel := false
	var parts []string
	for rest != "" {
		// Consume the gap between words. Titles are multi-word — "kaṅkhā
		// purāṇa abhinava ṭīkā", "vinayavinicchayo uttaravinicchayo" — and
		// without this the first word matched and the rest of the title was
		// carried through as untranslated Pāḷi.
		if n := len(rest) - len(strings.TrimLeft(rest, " ")); n > 0 {
			if len(parts) > 0 && !strings.HasPrefix(rest[n:], "(") {
				parts = append(parts, "·")
			}
			rest = rest[n:]
			continue
		}

		term, zh := matchTerm(rest, prevEndedInVowel)
		if term == "" {
			// No term matches here; carry the run of letters through
			// untranslated and resume matching after it.
			end := strings.IndexAny(rest, " ")
			if end < 0 {
				end = len(rest)
			}
			parts = append(parts, rest[:end])
			prevEndedInVowel = false
			rest = rest[end:]
			continue
		}
		// A term may map to nothing on purpose: "pāḷi" marks a text as the
		// canon rather than a commentary and has no separate Chinese word.
		if zh != "" {
			parts = append(parts, zh)
		}
		rest = rest[len(term):]
		// Titles are declined like anything else — vuttodayaṃ, and the -ṃ on
		// saddanītippakaraṇaṃ — so one bare case ending is consumed here. It is
		// only consumed when a word boundary follows, so a real word that
		// happens to start with the same letters is not eaten.
		rest = trimEnding(rest)
		prevEndedInVowel = strings.ContainsRune("aāiīuūeo", rune(term[len(term)-1]))
	}

	out := strings.Join(parts, "")
	if out == "" {
		out = name
	}
	if volume != "" {
		out += "（" + volume + "）"
	}
	return out
}

// caseEndings are the endings a declined title can carry. Longest first, so
// "āni" is not eaten as "ā" plus a stray "ni".
var caseEndings = []string{"āni", "aṃ", "ṃ", "o", "ā", "e", "ū", "ī", "i", "u"}

// trimEnding removes one bare case ending from the head of s, and only when a
// word boundary follows it.
func trimEnding(s string) string {
	for _, e := range caseEndings {
		if !strings.HasPrefix(s, e) {
			continue
		}
		after := s[len(e):]
		if after == "" || strings.HasPrefix(after, " ") || strings.HasPrefix(after, "(") {
			return after
		}
	}
	return s
}

// matchTerm finds the longest dictionary term that matches the head of rest,
// allowing for sandhi elision and for a nominative ending the dictionary form
// does not carry.
func matchTerm(rest string, prevEndedInVowel bool) (term, zh string) {
	for candidate := range termZh {
		if candidate == "" {
			continue
		}
		for _, form := range spellingVariants(candidate, prevEndedInVowel) {
			if strings.HasPrefix(rest, form) && len(form) > len(term) {
				term, zh = form, termZh[candidate]
			}
		}
	}
	return term, zh
}

// spellingVariants lists the forms a dictionary term can take in a running
// title: itself, its nominative singular, and — when the preceding word ended
// in a vowel — the elided form with its initial vowel absorbed.
//
// It returns the spellings to compare against, matched longest first so that a
// longer dictionary term always beats a shorter one that also fits.
func spellingVariants(term string, prevEndedInVowel bool) []string {
	out := []string{term}
	// Nominative singular: the source writes visuddhimaggo for visuddhimagga.
	switch {
	case strings.HasSuffix(term, "a"):
		out = append(out, term[:len(term)-1]+"o", term+"ṃ", term+"n")
	case strings.HasSuffix(term, "i") || strings.HasSuffix(term, "u"):
		out = append(out, term+"o", term+"ṃ")
	case strings.HasSuffix(term, "ā"):
		out = append(out, term[:len(term)-1]+"aṃ")
	}
	// Consonant doubling across a join: saddanīti + pakaraṇaṃ is written
	// saddanītippakaraṇaṃ. Only the first letter doubles, and only when it is
	// a consonant.
	if r, size := utf8.DecodeRuneInString(term); size > 0 && !strings.ContainsRune("aāiīuūeo", r) {
		out = append(out, string(r)+term)
	}
	// Elision: a final -a or -ā of the previous word absorbs an initial a- of
	// this one, so the term appears without its first letter.
	if prevEndedInVowel {
		if strings.HasPrefix(term, "a") || strings.HasPrefix(term, "ā") {
			out = append(out, term[1:])
		}
	}
	return out
}
