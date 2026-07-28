package db

import (
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github/musuyin/agent-weave/internal/config"
)

// ProvideDB opens a GORM connection and runs all pending migrations.
func ProvideDB(cfg *config.Config, log *slog.Logger) (*gorm.DB, func(), error) {
	dsn, err := mysqlDSN(cfg.Database.DatabaseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid database_url: %w", err)
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("gorm open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get sql.DB: %w", err)
	}

	if err := RunMigrations(sqlDB, log); err != nil {
		return nil, nil, fmt.Errorf("migrations: %w", err)
	}

	cleanup := func() {
		if err := sqlDB.Close(); err != nil {
			log.Error("db close", "error", err)
		}
	}
	return db, cleanup, nil
}

// mysqlDSN converts a "mysql://user:pass@tcp(host)/db" URL to a GORM DSN.
// Always ensures parseTime=true and loc=UTC are present.
func mysqlDSN(rawURL string) (string, error) {
	dsn := strings.TrimPrefix(rawURL, "mysql://")
	if dsn == rawURL {
		return "", fmt.Errorf("database_url must start with mysql://")
	}
	if strings.Contains(dsn, "?") {
		if !strings.Contains(dsn, "parseTime") {
			dsn += "&parseTime=true&loc=UTC"
		}
	} else {
		dsn += "?parseTime=true&charset=utf8mb4&loc=UTC"
	}
	return dsn, nil
}
