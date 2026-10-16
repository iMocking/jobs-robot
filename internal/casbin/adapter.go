package casbin

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"

	"github.com/jsong/jobs-robot/internal/pkg/ulidid"
)

// ruleColumns 是策略表承载的最大字段数（v0~v5），与 Casbin 官方表结构一致。
const ruleColumns = 6

// Adapter 是 Casbin 策略的 PostgreSQL 存储适配器。
//
// 自行实现而不用第三方 ent/gorm 适配器，原因有二：
//  1. 只需要一张扁平的 casbin_rule 表，不受 ORM 版本约束；
//  2. 避免为一个可选依赖引入 ent 版本冲突的风险。
type Adapter struct {
	db *sql.DB
}

// NewAdapter 构造策略适配器。
func NewAdapter(db *sql.DB) *Adapter { return &Adapter{db: db} }

// EnsureSchema 创建策略表（幂等）。
//
// 行标识同样使用 ULID 字符串而不是 BIGSERIAL：本项目不允许任何数字自增主键，
// 而且 ULID 前缀即时间戳，`ORDER BY id` 能稳定还原策略的插入顺序
// （角色继承与权限的加载顺序在排障时很有用）。
func (a *Adapter) EnsureSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS casbin_rule (
    id    VARCHAR(26) PRIMARY KEY,
    ptype VARCHAR(100) NOT NULL,
    v0    VARCHAR(256),
    v1    VARCHAR(256),
    v2    VARCHAR(256),
    v3    VARCHAR(256),
    v4    VARCHAR(256),
    v5    VARCHAR(256)
)`
	if _, err := a.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("创建 casbin_rule 表失败: %w", err)
	}
	if _, err := a.db.ExecContext(ctx,
		`CREATE INDEX IF NOT EXISTS idx_casbin_rule_ptype ON casbin_rule (ptype)`); err != nil {
		return fmt.Errorf("创建 casbin_rule 索引失败: %w", err)
	}
	return nil
}

// LoadPolicy 从数据库加载全部策略。
func (a *Adapter) LoadPolicy(m model.Model) error {
	rows, err := a.db.Query(
		`SELECT ptype, v0, v1, v2, v3, v4, v5 FROM casbin_rule ORDER BY id`)
	if err != nil {
		return fmt.Errorf("读取 casbin 策略失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ptype string
		values := make([]sql.NullString, ruleColumns)
		dest := []any{&ptype, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5]}
		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("解析 casbin 策略行失败: %w", err)
		}

		rule := []string{ptype}
		for _, v := range values {
			// 尾部空值一律截断，保证规则长度与策略定义匹配。
			if !v.Valid || v.String == "" {
				break
			}
			rule = append(rule, v.String)
		}
		if len(rule) <= 1 {
			continue
		}
		if err := persist.LoadPolicyArray(rule, m); err != nil {
			return fmt.Errorf("载入 casbin 策略失败 (%v): %w", rule, err)
		}
	}
	return rows.Err()
}

// SavePolicy 全量覆盖策略（先清空再写入）。
func (a *Adapter) SavePolicy(m model.Model) error {
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`TRUNCATE TABLE casbin_rule`); err != nil {
		return fmt.Errorf("清空 casbin 策略失败: %w", err)
	}

	insert := buildInsert(ruleColumns)
	stmt, err := tx.Prepare(insert)
	if err != nil {
		return fmt.Errorf("准备插入语句失败: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for ptype, ast := range m {
		for sec, rules := range ast {
			// 只持久化 p / g 段。
			if sec != "p" && sec != "g" {
				continue
			}
			for _, rule := range rules.Policy {
				if _, err := stmt.Exec(insertArgs(ptype, rule)...); err != nil {
					return fmt.Errorf("写入 casbin 策略失败 (%s %v): %w", ptype, rule, err)
				}
			}
		}
	}
	return tx.Commit()
}

// AddPolicy 追加一条策略（Auto-Save 时由 Casbin 调用）。
func (a *Adapter) AddPolicy(_ string, ptype string, rule []string) error {
	_, err := a.db.Exec(buildInsert(ruleColumns), insertArgs(ptype, rule)...)
	if err != nil {
		return fmt.Errorf("新增 casbin 策略失败 (%s %v): %w", ptype, rule, err)
	}
	return nil
}

// RemovePolicy 删除一条策略。
func (a *Adapter) RemovePolicy(_ string, ptype string, rule []string) error {
	where, args := buildWhere(ptype, 0, rule)
	_, err := a.db.Exec(`DELETE FROM casbin_rule WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("删除 casbin 策略失败 (%s %v): %w", ptype, rule, err)
	}
	return nil
}

// RemoveFilteredPolicy 按字段过滤删除策略。
func (a *Adapter) RemoveFilteredPolicy(_ string, ptype string, fieldIndex int, fieldValues ...string) error {
	where, args := buildWhere(ptype, fieldIndex, fieldValues)
	_, err := a.db.Exec(`DELETE FROM casbin_rule WHERE `+where, args...)
	if err != nil {
		return fmt.Errorf("按条件删除 casbin 策略失败 (%s): %w", ptype, err)
	}
	return nil
}

// buildInsert 生成参数化插入语句。
//
// 第一列是 ULID 行标识（由调用方生成），随后是 ptype 与 v0~v5。
func buildInsert(columns int) string {
	placeholders := make([]string, 0, columns)
	for i := 0; i < columns; i++ {
		placeholders = append(placeholders, fmt.Sprintf("$%d", i+3))
	}
	return fmt.Sprintf(
		`INSERT INTO casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5) VALUES ($1, $2, %s)`,
		strings.Join(placeholders, ", "))
}

// insertArgs 把规则补齐到固定列数；超过列数的部分直接截断（Casbin 官方行为一致）。
func insertArgs(ptype string, rule []string) []any {
	args := make([]any, 0, ruleColumns+2)
	args = append(args, ulidid.New(), ptype)
	for i := 0; i < ruleColumns; i++ {
		if i < len(rule) {
			args = append(args, rule[i])
			continue
		}
		args = append(args, nil)
	}
	return args
}

// buildWhere 生成 WHERE 子句：ptype + 从 fieldIndex 开始的若干 v 列。
func buildWhere(ptype string, fieldIndex int, values []string) (string, []any) {
	clauses := []string{"ptype = $1"}
	args := []any{ptype}
	for i, v := range values {
		col := fieldIndex + i
		if col >= ruleColumns {
			break
		}
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf("v%d = $%d", col, len(args)))
	}
	return strings.Join(clauses, " AND "), args
}
