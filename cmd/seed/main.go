// Command seed 初始化基础数据：角色元数据、初始管理员账号与内置配置项。
//
// 幂等：重复执行不会重复创建，也不会覆盖已存在管理员的密码。
//
// 用法：
//
//	go run ./cmd/seed -admin-email admin@example.com -admin-password 'YourPassw0rd'
//	go run ./cmd/seed            # 密码留空时自动生成并打印一次
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jsong/jobs-robot/internal/auth"
	"github.com/jsong/jobs-robot/internal/bootstrap"
	"github.com/jsong/jobs-robot/internal/casbin"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/role"
	entuser "github.com/jsong/jobs-robot/internal/ent/user"
	"github.com/jsong/jobs-robot/internal/service"
)

// roleSeed 是内置角色的展示元数据。
//
// 注意：这里只写「角色是什么」的描述信息；「角色能做什么」由 Casbin 策略负责，
// 两者分离可避免权限数据出现两份互相矛盾的来源。
type roleSeed struct {
	Name        string
	DisplayName string
	Description string
	Level       int
}

var roleSeeds = []roleSeed{
	{
		Name:        casbin.RoleAdmin,
		DisplayName: "管理员",
		Description: "拥有全部权限：管理公司、系统配置、用户与角色，并可按需调整审核结果",
		Level:       100,
	},
	{
		Name:        casbin.RoleReviewer,
		DisplayName: "审核员",
		Description: "审核爬取到的岗位（通过/驳回），只读查看公司与抓取记录",
		Level:       50,
	},
	{
		Name:        casbin.RoleUser,
		DisplayName: "用户",
		Description: "浏览已发布的直招岗位，维护个人资料与登录会话",
		Level:       10,
	},
	{
		Name:        casbin.RoleSystem,
		DisplayName: "系统",
		Description: "系统内置机器身份（爬虫 / 邮件机器人）：不继承任何角色、不能登录、不能访问后台接口，仅用于审计归属",
		// 低于普通用户：它没有任何业务权限，只是身份标记。
		Level: 1,
	},
}

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	adminEmail := flag.String("admin-email", "admin@jobs-robot.local", "初始管理员邮箱")
	adminPassword := flag.String("admin-password", "", "初始管理员密码（留空则自动生成并打印）")
	adminNickname := flag.String("admin-nickname", "系统管理员", "初始管理员昵称")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	app, err := bootstrap.Init(ctx, *configPath, bootstrap.Options{
		Migrate:      true,
		WithCrawler:  false,
		SeedSettings: true,
		// seed 自己要按 -admin-email / -admin-password 创建管理员，
		// 因此显式关闭启动自举，避免它抢先创建出另一个管理员账号。
		EnsureAdmin: false,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(1)
	}
	defer app.Close()

	if err := seedRoles(ctx, app); err != nil {
		fmt.Fprintln(os.Stderr, "写入角色失败:", err)
		os.Exit(1)
	}

	created, generated, err := seedAdmin(ctx, app, *adminEmail, *adminPassword, *adminNickname)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建管理员失败:", err)
		os.Exit(1)
	}

	// 汇总输出，便于部署脚本判断结果。
	roleCount, _ := app.Store.Ent.Role.Query().Count(ctx)
	// 用户数量只统计真人账号：系统机器身份由 bootstrap 幂等创建，不是运营数据。
	userCount, _ := app.Store.Ent.User.Query().
		Where(entuser.KindNEQ(entuser.KindSystem)).
		Count(ctx)
	fmt.Println()
	fmt.Println("========== 初始化完成 ==========")
	fmt.Printf("角色数量：%d\n", roleCount)
	fmt.Printf("用户数量：%d\n", userCount)
	fmt.Printf("管理员邮箱：%s\n", strings.ToLower(strings.TrimSpace(*adminEmail)))
	switch {
	case created && generated != "":
		fmt.Printf("管理员初始密码：%s\n", generated)
		fmt.Println("请立即登录并修改密码；该密码只显示这一次。")
	case created:
		fmt.Println("管理员已按你提供的密码创建。")
	default:
		fmt.Println("管理员此前已存在，密码未改动。")
	}
	fmt.Println("默认收集范围与角色策略已就绪，可通过 /api/v1/admin/crawl-scope 调整。")
	fmt.Println("================================")
}

// seedRoles 写入/更新内置角色元数据。
func seedRoles(ctx context.Context, app *bootstrap.App) error {
	for _, item := range roleSeeds {
		existing, err := app.Store.Ent.Role.Query().Where(role.NameEQ(item.Name)).Only(ctx)
		switch {
		case err == nil:
			// 已存在：同步展示信息，但不动 Casbin 中的实际策略。
			if _, err := app.Store.Ent.Role.UpdateOneID(existing.ID).
				SetDisplayName(item.DisplayName).
				SetDescription(item.Description).
				SetLevel(item.Level).
				SetIsSystem(true).
				Save(ctx); err != nil {
				return err
			}
			slog.Info("角色已更新", "name", item.Name)
		case ent.IsNotFound(err):
			if _, err := app.Store.Ent.Role.Create().
				SetName(item.Name).
				SetDisplayName(item.DisplayName).
				SetDescription(item.Description).
				SetLevel(item.Level).
				SetIsSystem(true).
				Save(ctx); err != nil {
				return err
			}
			slog.Info("角色已创建", "name", item.Name)
		default:
			return err
		}
	}
	return nil
}

// seedAdmin 创建初始管理员。
//
// 返回：是否新建、自动生成的明文密码（未生成时为空串）、错误。
func seedAdmin(ctx context.Context, app *bootstrap.App, email, password, nickname string) (bool, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, "", fmt.Errorf("管理员邮箱不能为空")
	}

	existing, err := app.Store.Ent.User.Query().Where(entuser.EmailEQ(email)).Only(ctx)
	if err == nil {
		// 已存在：只确保它拥有管理员角色，不改密码。
		roles, err := app.Authz.RolesForUser(existing.ID)
		if err != nil {
			return false, "", err
		}
		if !contains(roles, casbin.RoleAdmin) {
			if err := app.Authz.AssignRole(existing.ID, casbin.RoleAdmin); err != nil {
				return false, "", err
			}
			slog.Info("已为既有账号补充管理员角色", "email", email)
		}
		return false, "", nil
	}
	if !ent.IsNotFound(err) {
		return false, "", err
	}

	generated := ""
	if strings.TrimSpace(password) == "" {
		password, err = auth.GeneratePassword(16)
		if err != nil {
			return false, "", err
		}
		generated = password
	}
	if err := auth.ValidatePasswordStrength(password, app.Cfg.Auth.PasswordMinLength); err != nil {
		return false, "", fmt.Errorf("管理员密码不符合要求: %w", err)
	}

	hash, err := auth.HashPassword(password, app.Cfg.Auth.BcryptCost)
	if err != nil {
		return false, "", err
	}
	now := time.Now()

	user, err := app.Store.Ent.User.Create().
		SetEmail(email).
		SetPasswordHash(hash).
		SetNickname(strings.TrimSpace(nickname)).
		SetStatus(entuser.StatusActive).
		SetEmailVerified(true).
		SetEmailVerifiedAt(now).
		SetPasswordChangedAt(now).
		SetRegisterSource("seed").
		Save(ctx)
	if err != nil {
		return false, "", err
	}

	// 管理员同时具备审核员与普通用户能力（Casbin 中已配置角色继承）。
	if err := app.Authz.SetRoles(user.ID, []string{casbin.RoleAdmin}); err != nil {
		return false, "", err
	}

	app.Services.Audit.Record(ctx, service.SystemActor(), "user.seed_admin", "user",
		user.ID, map[string]any{"email": email})
	slog.Info("管理员账号已创建", "email", email, "user_id", user.ID)
	return true, generated, nil
}

// randomPassword 已提升为 auth.GeneratePassword（cmd/seed 与启动自举共用同一实现，
// 避免两处字符集或长度策略漂移）。

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
