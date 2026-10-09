package importer

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/metanoia/pali-reader/backend/internal/store"
	"github.com/metanoia/pali-reader/backend/internal/tokenize"
	"gorm.io/gorm/clause"
)

// Dictionary sources that are not DPD. Each is a separate voice in the lookup
// panel: PTS and the Concise dictionary give an English reading with a
// different editorial slant, the Chinese dictionaries give the reader a
// meaning in their own language, and DPPN covers the people and places that
// the general dictionaries only gloss.

// chineseSource is one Tabfile dictionary.
type chineseSource struct {
	File string
	Code string
	Name string
}

var chineseSources = []chineseSource{
	{"巴漢詞典_明法尊者.txt", "mingfa", "巴漢詞典 · 明法尊者增訂"},
	{"巴漢詞典_Mahāñāṇo.txt", "mahanano", "巴漢詞典 · Mahāñāṇo"},
	{"漢譯パーリ語辭典_黃秉榮.txt", "huang", "漢譯パーリ語辭典 · 黃秉榮譯"},
	{"パーリ語辞典_水野弘元.txt", "mizuno", "パーリ語辞典 · 水野弘元"},
}

// englishSource is one book of the dictionary table inside tipitaka_pali.db.
type englishSource struct {
	BookID int
	Code   string
	Name   string
}

var englishSources = []englishSource{
	{5, "pts", "PTS Pali-English Dictionary"},
	{6, "concise", "Concise Pali-English Dictionary"},
	{9, "dppn", "Pali Proper Names (DPPN)"},
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)
var wsRe = regexp.MustCompile(`[ \t\r\n]+`)

// stripHTML reduces a dictionary fragment to readable plain text. The source
// files carry presentational markup only; nothing is lost that the reader
// needs, and keeping HTML out of the column means the client renders glosses
// as text without an XSS surface.
func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "</li>", "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// ImportEntries loads every non-DPD dictionary.
func ImportEntries(tipitakaPath, zhDir, zhJSON string, g *store.DB, log func(string, ...any)) error {
	var sources []store.DictSource
	var entries []store.DictEntry
	sortOrder := 0

	add := func(code, name, lang, license string, rows map[string]string) {
		if len(rows) == 0 {
			return
		}
		sources = append(sources, store.DictSource{
			Code: code, Name: name, Lang: lang, License: license,
			Sort: sortOrder, Entries: len(rows),
		})
		sortOrder++
		keys := make([]string, 0, len(rows))
		for k := range rows {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			entries = append(entries, store.DictEntry{Word: k, Dict: code, Body: rows[k]})
		}
	}

	// --- Chinese tabfiles ---
	for _, src := range chineseSources {
		path := filepath.Join(zhDir, src.File)
		rows, err := readTabfile(path)
		if err != nil {
			log("  %s: %v", src.File, err)
			continue
		}
		log("  %-40s %6d entries", src.Name, len(rows))
		add(src.Code, src.Name, "zh", "见原书版权页", rows)
	}

	// --- the JSON supplement ---
	if rows, err := readZhJSON(zhJSON); err != nil {
		log("  %s: %v", filepath.Base(zhJSON), err)
	} else if len(rows) > 0 {
		log("  %-40s %6d entries", "巴利語中文詞典（社區整理）", len(rows))
		add("zhwiki", "巴利語中文詞典（社區整理）", "zh", "社区整理", rows)
	}

	// --- English books inside tipitaka_pali.db ---
	if tipitakaPath != "" {
		db, err := openSQLite(tipitakaPath)
		if err != nil {
			return err
		}
		defer db.Close()
		for _, src := range englishSources {
			rows, err := readDictionaryBook(db, src.BookID)
			if err != nil {
				log("  %s: %v", src.Name, err)
				continue
			}
			log("  %-40s %6d entries", src.Name, len(rows))
			add(src.Code, src.Name, "en", "见原书版权页", rows)
		}
	}

	if len(sources) == 0 {
		return nil
	}
	if err := g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(sources, 50).Error; err != nil {
		return err
	}
	log("  writing %d entries ...", len(entries))
	return g.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(entries, 1000).Error
}

// readTabfile reads a PyGlossary Tabfile: "#key=value" headers, then one
// "headword<TAB>definition" pair per line. Duplicate headwords are merged with
// a blank line between senses, which is how the source itself distinguishes
// homographs.
func readTabfile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		word, body, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		// Some dictionaries list several headwords on one line, separated by
		// "|" — "-ī|-i" means the entry answers to both spellings.
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		for _, w := range strings.Split(word, "|") {
			key := tokenize.Normalize(strings.TrimSpace(w))
			if key == "" {
				continue
			}
			if prev, ok := out[key]; ok && prev != body {
				out[key] = prev + "\n" + body
			} else {
				out[key] = body
			}
		}
	}
	return out, sc.Err()
}

// readZhJSON reads the community Chinese supplement, which is a flat array of
// {word, definition} records.
func readZhJSON(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recs []struct {
		Word       string `json:"word"`
		Definition string `json:"definition"`
	}
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range recs {
		// A record may carry several comma-separated headwords.
		for _, w := range strings.Split(r.Word, ",") {
			key := tokenize.Normalize(strings.TrimSpace(w))
			if key == "" {
				continue
			}
			body := stripHTML(r.Definition)
			if body == "" {
				continue
			}
			if prev, ok := out[key]; ok && prev != body {
				out[key] = prev + "\n" + body
			} else {
				out[key] = body
			}
		}
	}
	return out, nil
}

func readDictionaryBook(db *sql.DB, bookID int) (map[string]string, error) {
	rows, err := db.Query(`SELECT word, COALESCE(definition,'') FROM dictionary WHERE book_id = ?`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var word, def string
		if err := rows.Scan(&word, &def); err != nil {
			return nil, err
		}
		body := stripHTML(def)
		if body == "" {
			continue
		}
		// Concise PED separates senses with "/"; keep them, they read well.
		for _, w := range strings.Split(word, ",") {
			key := tokenize.Normalize(strings.TrimSpace(w))
			if key == "" {
				continue
			}
			if prev, ok := out[key]; ok && prev != body {
				out[key] = prev + "\n" + body
			} else {
				out[key] = body
			}
		}
	}
	return out, rows.Err()
}
