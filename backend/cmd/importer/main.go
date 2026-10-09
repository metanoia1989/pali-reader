// Command importer builds the Pāḷi Tipiṭaka reader's database from upstream
// sources. It is a build-time tool: the server never writes corpus or
// dictionary rows.
//
// Typical use, from backend/:
//
//	go run ./cmd/importer -sources ../data/sources -steps all
//
// Steps are independent, and each is idempotent, so a failed run can be
// resumed with -steps <name>.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/metanoia/pali-reader/backend/internal/config"
	"github.com/metanoia/pali-reader/backend/internal/importer"
	"github.com/metanoia/pali-reader/backend/internal/store"

	_ "modernc.org/sqlite"
)

func main() {
	log.SetFlags(log.Ltime)
	var (
		sources = flag.String("sources", "../data/sources", "directory holding the upstream databases")
		steps   = flag.String("steps", "all", "comma separated: schema,dict,text,catalog,entries,ref,freq,report,all")
		minCov  = flag.Float64("ref-min-coverage", 0.15, `skip a book whose reference alignment places less than this.

The number is the share of the lines its mapped volumes could contribute that
were actually placed: the anchored share of the volume's lines, divided by the
share of the volume's words this book is large enough to hold. The divisor is
the point. A volume several times the size of the book holds lines that are not
the book's text at all and no alignment can place them, so scoring against a
hundred per cent calls a correct mapping wrong. It did: mula_ku_01 (the
Khuddakapāṭha, 1,255 words, whose map also lists Sn's 21,000) and attha_sa_05
(the Mahāvagga commentary, 30,660 words, whose map also lists Ps-i's 137,710)
anchored nearly every line of the volume that is theirs and were thrown away
whole, because their map lists one volume too many.

A line is only written when it anchored, so a book at 30% contributes those 30%
and nothing for the rest.`)
		drop = flag.Bool("drop", false, "drop every corpus and dictionary table first")
	)
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal(err)
	}
	dir, err := filepath.Abs(*sources)
	if err != nil {
		fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		fatal(fmt.Errorf("sources directory %s: %w", dir, err))
	}

	g, err := store.OpenQuiet(cfg.MySQLDSN)
	if err != nil {
		fatal(err)
	}

	want := map[string]bool{}
	for _, s := range strings.Split(*steps, ",") {
		want[strings.TrimSpace(s)] = true
	}
	all := want["all"]
	do := func(name string) bool { return all || want[name] }

	start := time.Now()
	step := func(name string, fn func() error) {
		if !do(name) {
			return
		}
		t := time.Now()
		log.Printf("== %s", name)
		if err := fn(); err != nil {
			fatal(fmt.Errorf("%s: %w", name, err))
		}
		log.Printf("== %s done in %s", name, time.Since(t).Round(time.Millisecond))
	}

	src := func(name string) string { return filepath.Join(dir, name) }

	if *drop {
		log.Printf("== dropping corpus and dictionary tables")
		if err := dropTables(g); err != nil {
			fatal(err)
		}
	}

	step("schema", func() error { return g.Migrate() })

	step("dict", func() error {
		return importer.ImportDictionary(src("dpd.db"), src("tipitaka_pali.db"), g, log.Printf)
	})

	step("text", func() error {
		return importer.ImportText(src("tipitaka_pali.db"), src("epitaka.db"), g, log.Printf)
	})

	step("catalog", func() error {
		return importer.ImportCatalog(src("tipitaka_pali.db"), g, log.Printf)
	})

	step("entries", func() error {
		return importer.ImportEntries(
			src("tipitaka_pali.db"),
			filepath.Join(dir, "zh"),
			filepath.Join(dir, "zh_supplement.json"),
			g, log.Printf)
	})

	step("ref", func() error {
		return importer.ImportReferenceTranslations(src("epitaka.db"), []importer.RefSource{
			{Lang: "zh", Path: src("epitaka_zh.db")},
			{Lang: "en", Path: src("epitaka_en.db")},
		}, g, *minCov, log.Printf)
	})

	step("freq", func() error { return importer.ImportWordFrequency(g, log.Printf) })

	// Read-only, and about the quality of what was written rather than the
	// writing of it: how many sentences of translation ended up under one
	// segment. Run before and after an import whose effect is being argued.
	step("report", func() error {
		return importer.ReportStacking(g, []string{"zh", "en"}, log.Printf)
	})

	log.Printf("all done in %s", time.Since(start).Round(time.Second))
}

// dropTables removes the imported tables so -drop gives a genuinely clean
// rebuild. User tables are deliberately left alone.
func dropTables(g *store.DB) error {
	names := []string{
		"text_segments", "text_toc", "text_books", "text_categories",
		"dict_lookup", "dict_headwords", "dict_templates", "dict_roots",
		"dict_entries", "dict_sources", "ref_translations", "word_freq",
	}
	// AutoMigrate adds columns but will not alter an existing column's
	// collation, and the dictionary key columns changed to utf8mb4_bin when it
	// turned out the default collation was folding ḍ onto d. Dropping is the
	// only way to land the fix, and this is a rebuild path anyway.
	for _, n := range names {
		if err := g.Exec("DROP TABLE IF EXISTS " + n).Error; err != nil {
			return err
		}
	}
	return nil
}

func fatal(err error) {
	log.SetFlags(0)
	log.Fatalf("importer: %v", err)
}
