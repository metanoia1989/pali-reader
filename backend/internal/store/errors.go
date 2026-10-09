package store

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// ErrNotFound is returned by the helpers below instead of gorm.ErrRecordNotFound
// so handlers do not import GORM.
var ErrNotFound = gorm.ErrRecordNotFound

// IsNotFound reports whether err means "no such row".
func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

// IsDuplicate reports whether err is a unique-constraint violation. It checks
// both the driver error number and the message, because GORM sometimes wraps
// the driver error in a way that loses the number.
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number == 1062
	}
	return strings.Contains(err.Error(), "Error 1062")
}

func isDuplicateIndex(err error) bool {
	if err == nil {
		return false
	}
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		// 1061 = duplicate key name
		return me.Number == 1061
	}
	return strings.Contains(err.Error(), "Error 1061") ||
		strings.Contains(err.Error(), "Duplicate key name")
}
