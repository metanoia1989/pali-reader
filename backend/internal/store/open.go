package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// DB wraps the GORM handle so callers depend on this package rather than on
// GORM directly.
type DB struct {
	*gorm.DB
}

// Open connects to MySQL with GORM's normal logging.
func Open(dsn string, dev bool) (*DB, error) { return open(dsn, gormlogger.Warn) }

// OpenQuiet connects with SQL logging off. The importer writes millions of
// rows and prints its own progress; GORM's per-error SQL dump would bury it.
func OpenQuiet(dsn string) (*DB, error) { return open(dsn, gormlogger.Silent) }

func open(dsn string, level gormlogger.LogLevel) (*DB, error) {
	logger := gormlogger.New(log.New(os.Stdout, "[sql] ", log.LstdFlags), gormlogger.Config{
		SlowThreshold:             300 * time.Millisecond,
		LogLevel:                  level,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
		// Never print bound values: the corpus is large and a failed batch
		// would otherwise dump thousands of rows into the log.
		ParameterizedQueries: true,
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       dsn,
		DefaultStringSize:         191,  // MySQL 5.7 safe index width
		DisableDatetimePrecision:  true, // 5.7 has no fractional seconds by default
		DontSupportRenameIndex:    true, // 5.7 cannot rename an index in place
		DontSupportRenameColumn:   true, // nor a column
		DontSupportForShareClause: true, // nor FOR SHARE
		SkipInitializeWithVersion: false,
	}), &gorm.Config{
		Logger: logger,
		NamingStrategy: schema.NamingStrategy{
			SingularTable: false,
		},
		// PrepareStmt is off on purpose: the importer issues millions of
		// statements across a handful of shapes, and caching them exhausted
		// MySQL's max_prepared_stmt_count. The runtime queries are all
		// primary-key or narrow-index lookups and gain little from it.
		PrepareStmt:            false,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("connect mysql: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(40)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return &DB{db}, nil
}

// SQL exposes the connection pool, for health checks and shutdown.
func (d *DB) SQL() (*sql.DB, error) { return d.DB.DB() }

// Migrate creates or updates every table the application owns.
func (d *DB) Migrate() error {
	models := []any{
		&TextCategory{}, &TextBook{}, &TextSegment{}, &TextTOC{},
		&DictHeadword{}, &DictLookup{}, &DictTemplate{}, &DictRoot{},
		&DictEntry{}, &DictSource{},
		&RefTranslation{},
		&User{}, &PendingRegistration{}, &Session{},
		&WordPick{}, &Note{}, &Translation{}, &Progress{}, &Bookmark{},
		&VocabItem{}, &WordFreq{},
		// Dropped rather than migrated: the shape of a word annotation changed
		// from one decision per word to any number of them, and an old row
		// cannot be reinterpreted as the new thing.
		&retiredWordChoice{},
	}
	for _, m := range models {
		if err := d.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate %T: %w", m, err)
		}
	}
	return d.postMigrate()
}

// postMigrate adds the few indexes GORM's tags cannot express: composite
// covering indexes that the reader's hottest queries rely on.
//
// Existence is checked first rather than relying on "duplicate key name" being
// returned and swallowed: that works, but it makes every start-up log an error
// that is not one, and a real failure then hides among the noise.
func (d *DB) postMigrate() error {
	stmts := []struct{ table, index, ddl string }{
		// Segment fetch: the reader always asks for a contiguous run of one book.
		{"text_segments", "idx_segment_book_seq",
			`CREATE INDEX idx_segment_book_seq ON text_segments (book_id, seq)`},
		// Word lookup by lemma prefix, for the search box.
		{"dict_headwords", "idx_headword_lemma_lower",
			`CREATE INDEX idx_headword_lemma_lower ON dict_headwords (lemma_1)`},
	}
	for _, s := range stmts {
		var n int64
		if err := d.Raw(`SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
			s.table, s.index).Scan(&n).Error; err != nil {
			return fmt.Errorf("check index %s: %w", s.index, err)
		}
		if n > 0 {
			continue
		}
		if err := d.Exec(s.ddl).Error; err != nil && !isDuplicateIndex(err) {
			return fmt.Errorf("%s: %w", s.ddl, err)
		}
	}
	return nil
}
