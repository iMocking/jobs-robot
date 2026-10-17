// Package store 负责创建并持有 PostgreSQL（ent）与 Redis 连接。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // 注册 pgx database/sql 驱动
	"github.com/redis/go-redis/v9"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/migrate"
)

// Store 聚合数据访问客户端。
type Store struct {
	Ent   *ent.Client
	Redis *redis.Client

	sqlDB   *sql.DB
	prefix  string
	closeFn []func() error
}

// Open 建立 PostgreSQL 与 Redis 连接，任一不可用即返回错误（快速失败，避免带病启动）。
func Open(ctx context.Context, cfg *config.Config) (*Store, error) {
	sqlDB, err := sql.Open("pgx", cfg.Database.DSN())
	if err != nil {
		return nil, fmt.Errorf("打开 PostgreSQL 连接失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("PostgreSQL 不可用（%s:%d/%s）: %w",
			cfg.Database.Host, cfg.Database.Port, cfg.Database.Name, err)
	}

	entClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addr,
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	})
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = sqlDB.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("Redis 不可用（%s）: %w", cfg.Redis.Addr, err)
	}

	slog.Info("数据存储连接就绪",
		"postgres", fmt.Sprintf("%s:%d/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.Name),
		"redis", cfg.Redis.Addr,
	)

	return &Store{
		Ent:    entClient,
		Redis:  rdb,
		sqlDB:  sqlDB,
		prefix: cfg.Redis.KeyPrefix,
		closeFn: []func() error{
			func() error { return rdb.Close() },
			func() error { return entClient.Close() },
		},
	}, nil
}

// SQL 暴露底层 *sql.DB，便于执行少量原生 SQL（如健康检查）。
func (s *Store) SQL() *sql.DB { return s.sqlDB }

// Key 生成带统一前缀的 Redis 键，避免与其他应用共库时冲突。
func (s *Store) Key(parts ...string) string {
	return s.prefix + strings.Join(parts, ":")
}

// Migrate 执行 ent 自动迁移。
func (s *Store) Migrate(ctx context.Context) error {
	if err := s.Ent.Schema.Create(ctx,
		migrate.WithDropIndex(false),
		migrate.WithDropColumn(false),
	); err != nil {
		return fmt.Errorf("执行数据库迁移失败: %w", err)
	}
	if err := s.ensurePartialUniqueIndexes(ctx); err != nil {
		return err
	}
	slog.Info("数据库迁移完成")
	return nil
}

// ensurePartialUniqueIndexes 清理「邮箱全表唯一」的旧索引。
//
// 背景：users.email 改为 PostgreSQL 部分唯一索引（where email <> ”），
// 因为系统机器身份（爬虫、邮件机器人）的 email 为空，全表唯一索引
// 只允许存在一个空串，第二个系统身份就插不进去。
//
// ent 生成的 user_email / user_system_key 已经是部分唯一索引；但旧库里的
// users_email_key（field.Unique() 的产物）不会因为 WithDropIndex(false) 被删除，
// 必须在这里显式清掉，否则系统身份插入会撞上它。语句幂等，重复执行无副作用。
func (s *Store) ensurePartialUniqueIndexes(ctx context.Context) error {
	legacy := []string{
		`DROP INDEX IF EXISTS users_email_key`,
		`DROP INDEX IF EXISTS users_system_key_key`,
	}
	for _, stmt := range legacy {
		if _, err := s.sqlDB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("清理旧唯一索引失败（%s）: %w", stmt, err)
		}
	}
	return nil
}

// Health 检查两个依赖是否可用。
func (s *Store) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	if err := s.Redis.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	return nil
}

// Close 释放所有连接。
func (s *Store) Close() error {
	var firstErr error
	for _, fn := range s.closeFn {
		if err := fn(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
