package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"reflect"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/jsong/jobs-robot/internal/config"
	"github.com/jsong/jobs-robot/internal/crawler"
	"github.com/jsong/jobs-robot/internal/ent"
	"github.com/jsong/jobs-robot/internal/ent/setting"
	"github.com/jsong/jobs-robot/internal/pkg/apperr"
)

// SettingKeyCrawlScope 是爬虫收集范围的配置键。
const SettingKeyCrawlScope = "crawler.scope"

// 配置项的内置元信息与审计动作标识。
const (
	settingGroupCrawler          = "crawler"
	settingCrawlScopeDescription = "爬虫收集的工作类型与行业范围"

	actionSettingCreate = "setting.create"
	actionSettingUpdate = "setting.update"
	actionSettingDelete = "setting.delete"

	resourceSetting = "setting"
)

// SettingService 负责运行时热更新配置（键值 + JSON 内容）。
//
// 存在意义：抓取范围（工作类型/行业白名单）这类参数需要管理员随时调整并立即生效，
// 落库后无需重启服务，因此统一以 map[string]any 存储。
type SettingService struct{ s *Services }

// List 查询配置项；group 为空时返回全部，按分组与键名排序便于后台展示。
func (svc *SettingService) List(ctx context.Context, group string) ([]SettingView, error) {
	q := svc.s.Store.Ent.Setting.Query()
	if g := strings.TrimSpace(group); g != "" {
		q = q.Where(setting.GroupEQ(g))
	}

	rows, err := q.Order(setting.ByGroup(sql.OrderAsc()), setting.ByKey(sql.OrderAsc())).All(ctx)
	if err != nil {
		return nil, apperr.ErrInternal.WithCause(err)
	}

	views := make([]SettingView, 0, len(rows))
	for _, row := range rows {
		views = append(views, NewSettingView(row))
	}
	return views, nil
}

// Get 按键读取配置项。
func (svc *SettingService) Get(ctx context.Context, key string) (SettingView, error) {
	row, err := svc.s.Store.Ent.Setting.Query().
		Where(setting.KeyEQ(strings.TrimSpace(key))).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return SettingView{}, apperr.ErrSettingNotFound.WithCause(err)
		}
		return SettingView{}, apperr.ErrInternal.WithCause(err)
	}
	return NewSettingView(row), nil
}

// Upsert 新建或覆盖配置项。
//
// 语义约定：description / group 传入空串表示「不修改已有值」，
// 避免只想改 value 的调用方不小心清掉配置说明；新建时按传入值落库。
// is_system 只能由内部流程（如 SaveCrawlScope）提升，普通 Upsert 不会把内置项降级。
func (svc *SettingService) Upsert(ctx context.Context, key string, value map[string]any, description, group string, actor Actor) (SettingView, error) {
	return svc.upsert(ctx, key, value, description, group, false, actor)
}

// upsert 是 Upsert 与 SaveCrawlScope 的共同实现。
func (svc *SettingService) upsert(ctx context.Context, key string, value map[string]any, description, group string, isSystem bool, actor Actor) (SettingView, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return SettingView{}, apperr.ErrSettingInvalid.
			WithMessage("配置键不能为空").
			WithDetails(map[string]any{"field": "key"})
	}
	if len([]rune(key)) > 128 {
		return SettingView{}, apperr.ErrSettingInvalid.
			WithMessage("配置键过长（最多 128 字符）").
			WithDetails(map[string]any{"field": "key", "length": len([]rune(key))})
	}
	if value == nil {
		// JSON 字段统一存空对象，读出来仍是 map，避免调用方到处判 nil。
		value = map[string]any{}
	}

	updatedBy := truncateRunes(actor.Email, 255)

	current, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(key)).Only(ctx)
	switch {
	case err == nil:
		update := svc.s.Store.Ent.Setting.UpdateOneID(current.ID).
			SetValue(value).
			SetUpdatedBy(updatedBy)
		detail := map[string]any{}

		if !reflect.DeepEqual(current.Value, value) {
			detail["value"] = fieldChange(current.Value, value)
		}
		if description != "" && description != current.Description {
			update = update.SetDescription(description)
			detail["description"] = fieldChange(current.Description, description)
		}
		if group != "" && group != current.Group {
			update = update.SetGroup(group)
			detail["group"] = fieldChange(current.Group, group)
		}
		if isSystem && !current.IsSystem {
			update = update.SetIsSystem(true)
			detail["is_system"] = fieldChange(false, true)
		}

		if len(detail) == 0 {
			return NewSettingView(current), nil
		}

		saved, err := update.Save(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return SettingView{}, apperr.ErrSettingNotFound.WithCause(err)
			}
			return SettingView{}, apperr.ErrInternal.WithCause(err)
		}
		svc.s.Audit.Record(ctx, actor, actionSettingUpdate, resourceSetting, key, detail)
		return NewSettingView(saved), nil

	case ent.IsNotFound(err):
		created, err := svc.s.Store.Ent.Setting.Create().
			SetKey(key).
			SetValue(value).
			SetDescription(description).
			SetGroup(group).
			SetIsSystem(isSystem).
			SetUpdatedBy(updatedBy).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				// 并发创建同键：提示重试即可，无需在这里做 upsert 重试循环。
				return SettingView{}, apperr.ErrSettingInvalid.
					WithMessage("配置项写入冲突，请重试").
					WithCause(err)
			}
			return SettingView{}, apperr.ErrInternal.WithCause(err)
		}
		svc.s.Audit.Record(ctx, actor, actionSettingCreate, resourceSetting, key, map[string]any{
			"value":     created.Value,
			"group":     created.Group,
			"is_system": created.IsSystem,
		})
		return NewSettingView(created), nil

	default:
		return SettingView{}, apperr.ErrInternal.WithCause(err)
	}
}

// Delete 删除配置项；系统内置项（is_system=true）不允许删除。
//
// 内置项被删会导致依赖它的功能（如抓取范围）静默回退到默认值，
// 这类「配置凭空消失」的故障极难定位，因此宁可拒绝。
func (svc *SettingService) Delete(ctx context.Context, key string, actor Actor) error {
	key = strings.TrimSpace(key)

	current, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(key)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return apperr.ErrSettingNotFound.WithCause(err)
		}
		return apperr.ErrInternal.WithCause(err)
	}
	if current.IsSystem {
		return apperr.ErrSettingInvalid.
			WithMessage("系统内置配置不允许删除").
			WithDetails(map[string]any{"key": current.Key})
	}

	if err := svc.s.Store.Ent.Setting.DeleteOneID(current.ID).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return apperr.ErrSettingNotFound.WithCause(err)
		}
		return apperr.ErrInternal.WithCause(err)
	}

	svc.s.Audit.Record(ctx, actor, actionSettingDelete, resourceSetting, key, map[string]any{
		"value": current.Value,
		"group": current.Group,
	})
	return nil
}

// CrawlScope 返回「最终生效」的收集范围。
//
// 合并规则：以 config 中的默认值为基线，被数据库中的 crawler.scope 逐字段覆盖；
// 数据库里缺失的字段保持默认值，显式写成空数组则表示「取消该维度限制」。
//
// 容错取舍：脏数据（类型不符）只告警并沿用默认值，不返回错误。
// 原因是该方法会被定时抓取任务直接调用——保存侧（SaveCrawlScope）已是强类型，
// 出现脏数据只可能是人工直改数据库，此时让整轮抓取全部失败得不偿失。
func (svc *SettingService) CrawlScope(ctx context.Context) (crawler.Scope, error) {
	scope := crawler.Scope{}
	if svc.s.Cfg != nil {
		scope = configScopeToCrawler(svc.s.Cfg.Crawler.Scope)
	}

	row, err := svc.s.Store.Ent.Setting.Query().Where(setting.KeyEQ(SettingKeyCrawlScope)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		return scope, nil
	case err != nil:
		return scope, apperr.ErrInternal.WithCause(err)
	}

	mergeScopeMap(&scope, row.Value)
	return scope, nil
}

// SaveCrawlScope 保存收集范围。
//
// 键名统一用 snake_case，与 schema 注释中的示例保持一致；
// 该配置为系统内置项（is_system=true），因此不允许通过 Delete 删除。
func (svc *SettingService) SaveCrawlScope(ctx context.Context, scope crawler.Scope, actor Actor) (SettingView, error) {
	if scope.MinQualityScore < 0 {
		return SettingView{}, apperr.ErrSettingInvalid.
			WithMessage("最低质量分不能为负数").
			WithDetails(map[string]any{"field": "min_quality_score", "value": scope.MinQualityScore})
	}
	return svc.upsert(
		ctx,
		SettingKeyCrawlScope,
		scopeToMap(scope),
		settingCrawlScopeDescription,
		settingGroupCrawler,
		true,
		actor,
	)
}

// EnsureDefaults 播种内置配置项（幂等），服务启动时调用。
//
// 已存在的配置一律跳过，绝不覆盖——管理员改过的值优先级高于代码默认值。
func (svc *SettingService) EnsureDefaults(ctx context.Context, actor Actor) error {
	if actor.UserID == "" && strings.TrimSpace(actor.Email) == "" {
		actor = SystemActor()
	}

	// 目前仅 crawler.scope 一项内置配置；后续新增项在此追加即可。
	if _, err := svc.Get(ctx, SettingKeyCrawlScope); err == nil {
		return nil
	} else if !apperr.Is(err, apperr.CodeSettingNotFound) {
		return err
	}

	scope := crawler.Scope{}
	if svc.s.Cfg != nil {
		scope = configScopeToCrawler(svc.s.Cfg.Crawler.Scope)
	}

	_, err := svc.upsert(
		ctx,
		SettingKeyCrawlScope,
		scopeToMap(scope),
		settingCrawlScopeDescription,
		settingGroupCrawler,
		true,
		actor,
	)
	return err
}

// —— 收集范围的序列化与合并 ——

// configScopeToCrawler 把静态配置的 Scope 转成爬虫使用的 Scope。
//
// 两者字段名相同但是不同类型（配置包不该反向依赖爬虫包），因此显式转换。
func configScopeToCrawler(scope config.Scope) crawler.Scope {
	return crawler.Scope{
		JobTypes:        scope.JobTypes,
		Industries:      scope.Industries,
		EmploymentTypes: scope.EmploymentTypes,
		Locations:       scope.Locations,
		Keywords:        scope.Keywords,
		ExcludeKeywords: scope.ExcludeKeywords,
		MinQualityScore: scope.MinQualityScore,
	}
}

// scopeToMap 把 Scope 序列化成规范化的 map，键名为 snake_case。
func scopeToMap(scope crawler.Scope) map[string]any {
	return map[string]any{
		"job_types":         nonNilStrings(scope.JobTypes),
		"industries":        nonNilStrings(scope.Industries),
		"employment_types":  nonNilStrings(scope.EmploymentTypes),
		"locations":         nonNilStrings(scope.Locations),
		"keywords":          nonNilStrings(scope.Keywords),
		"exclude_keywords":  nonNilStrings(scope.ExcludeKeywords),
		"min_quality_score": scope.MinQualityScore,
	}
}

// nonNilStrings 把 nil 切片规范化为空切片：JSON 序列化时是 [] 而不是 null，
// 前端与其它读方不必再区分两种「空」。
func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// mergeScopeMap 用数据库中的配置覆盖 scope；缺失或类型不符的字段保持原值。
func mergeScopeMap(scope *crawler.Scope, values map[string]any) {
	for key, value := range values {
		switch key {
		case "job_types":
			setStringList(&scope.JobTypes, key, value)
		case "industries":
			setStringList(&scope.Industries, key, value)
		case "employment_types":
			setStringList(&scope.EmploymentTypes, key, value)
		case "locations":
			setStringList(&scope.Locations, key, value)
		case "keywords":
			setStringList(&scope.Keywords, key, value)
		case "exclude_keywords":
			setStringList(&scope.ExcludeKeywords, key, value)
		case "min_quality_score":
			if n, ok := toIntValue(value); ok {
				scope.MinQualityScore = n
			} else {
				slog.Warn("crawler.scope.min_quality_score 类型不正确，已沿用配置默认值",
					"value", value)
			}
		default:
			// 未知键多为历史残留或拼写错误，忽略即可，但要留痕以便清理。
			slog.Warn("crawler.scope 含未知键，已忽略", "key", key, "value", value)
		}
	}
}

// setStringList 写入字符串数组字段，类型不符时告警并保持原值。
func setStringList(target *[]string, key string, value any) {
	list, ok := toStringSlice(value)
	if !ok {
		slog.Warn("crawler.scope 字段类型不正确，已沿用配置默认值",
			"key", key, "value", value)
		return
	}
	*target = list
}

// —— JSON 值宽松转换（company.go 的 crawl_rules 校验同样复用） ——

// toStringSlice 把 JSON 反序列化后的值转成字符串数组。
//
// 兼容 []string（代码内直接构造）与 []any（JSON 反序列化结果）。
func toStringSlice(value any) ([]string, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case []string:
		return append([]string{}, v...), true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

// toStringMap 把值转成「字符串 → 字符串」的对象，用于 detail_fields。
func toStringMap(value any) (map[string]string, bool) {
	switch v := value.(type) {
	case nil:
		return nil, false
	case map[string]string:
		out := make(map[string]string, len(v))
		for k, val := range v {
			out[k] = val
		}
		return out, true
	case map[string]any:
		out := make(map[string]string, len(v))
		for k, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out[k] = s
		}
		return out, true
	default:
		return nil, false
	}
}

// toIntValue 把值转成整数。
//
// JSON 反序列化后数字是 float64，直接断言 int 会失败，因此这里统一兼容：
// float64 / int / int64 / json.Number / 纯数字字符串。
func toIntValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		if f := float64(v); f == math.Trunc(f) {
			return int(f), true
		}
		return 0, false
	case float64:
		// 只接受整数值：1.5 这种明显是配置写错，不应被静默截断。
		if v == math.Trunc(v) {
			return int(v), true
		}
		return 0, false
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n), true
		}
		if f, err := v.Float64(); err == nil && f == math.Trunc(f) {
			return int(f), true
		}
		return 0, false
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, true
		}
		return 0, false
	default:
		return 0, false
	}
}
