package data

import (
	"bytes"
	"database/sql"
	"github.com/uk0/silk/core"
	_ "github.com/mattn/go-sqlite3"
	"runtime"
	"strings"
	"sync"
)

var dbMutex sync.Mutex

type DB struct {
	p *sql.DB

	driverName     string
	dataSourceName string
}

func (db *DB) String() string {
	if db.driverName == "" {
		return "DB : uinitialized"
	}

	if db.p == nil {
		return `DB (closed) : "` + db.driverName + `" : "` + db.dataSourceName + `"`
	} else {
		return `DB (opened) : "` + db.driverName + `" : "` + db.dataSourceName + `"`
	}
}

func (db *DB) Close() {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if db.p != nil {
		db.p.Close()
		db.p = nil
		core.Trace(db)
	}
}

// 打开数据库
func OpenDB(driverName, dataSourceName string) (*DB, error) {
	p, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	db := new(DB)
	db.p = p
	db.driverName = driverName
	db.dataSourceName = dataSourceName
	runtime.SetFinalizer(db, (*DB).Close)
	core.Trace(db)
	return db, nil
}

// 分割多条SQL, 各SQL之间以分号';'分隔
func splitMultiSql(s string) (ret []string) {
	sz := len(s)
	buf := &bytes.Buffer{}
	var quote byte
	for i := 0; i < sz; i++ {
		ch := s[i]
		buf.WriteByte(ch)
		switch quote {
		case '"':
			if ch == '"' {
				if i+1 < sz && s[i+1] == '"' {
					buf.WriteByte(s[i+1])
					i++
				} else {
					quote = 0
				}
			}
		case '\'':
			if ch == '\'' {
				if i+1 < sz && s[i+1] == '\'' {
					buf.WriteByte(s[i+1])
					i++
				} else {
					quote = 0
				}
			}
		default:
			switch ch {
			case '"':
				quote = ch
			case '\'':
				quote = ch
			case ';':
				line := strings.TrimFunc(buf.String(), func(r rune) bool { return r <= 32 })
				if len(line) > 1 { // 跳过空行或者只有分号的行
					ret = append(ret, line)
				}
				buf.Reset()
			}
		}
	}
	if buf.Len() > 0 {
		line := strings.TrimFunc(buf.String(), func(r rune) bool { return r <= 32 })
		if len(line) > 0 { // 跳过空行
			ret = append(ret, line)
		}
	}
	return
}

func isTransactionCmd(s string) bool {
	sz := len(s)
	if sz >= 6 &&
		(s[0] == 'C' || s[0] == 'c') &&
		(s[1] == 'O' || s[1] == 'o') &&
		strings.ToLower(s[:6]) == "commit" {
		return true
	}

	if sz >= 5 &&
		(s[0] == 'B' || s[0] == 'b') &&
		strings.ToLower(s[:5]) == "begin" {
		return true
	}
	return false
}

func (db *DB) Exec(s string) (sql.Result, error) {
	if core.IsDebugOn() {
		core.Trace("DB.Exec : ", s)
		r, err := db.p.Exec(s)
		if err != nil {
			core.Warn(err)
		}
		return r, err
	} else {
		return db.p.Exec(s)
	}

}

// 批量执行多条SQL, 各SQL之间以分号';'分隔
func (db *DB) BatchExec(multiSql string, autoTransaction, breakOnError bool) error {
	return db.BatchExec1(splitMultiSql(multiSql), autoTransaction, breakOnError)
}

func (db *DB) BatchExec1(multiSql []string, autoTransaction, breakOnError bool) error {
	if len(multiSql) == 0 {
		return nil
	}
	transaction := false
	if autoTransaction && len(multiSql) > 1 {
		_, err := db.Exec("BEGIN TRANSACTION")
		if err != nil {
			transaction = true
		}
	}
	for _, a := range multiSql {
		// 如果是事务模式, 则忽略内嵌的事务处理语句
		if transaction && isTransactionCmd(a) {
			continue
		}
		_, err := db.Exec(a)
		if err != nil && breakOnError {
			if transaction {
				db.Exec("ROLLBACK")
			}
			return err
		}
	}
	if transaction {
		_, err := db.Exec("COMMIT")
		return err
	}
	return nil
}
