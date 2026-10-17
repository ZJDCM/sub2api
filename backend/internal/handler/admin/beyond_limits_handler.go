package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// BeyondLimitsHandler 破甲管道（beyond limits）后台接口：
//   - 按平台（openai / deepseek / gemini）读取/更新管道配置（bridge + 篡改规则）
//   - 分组级开关（勾选分组 → 对应平台的 beyond_limits_*_enabled）
//   - 监控统计与交互日志
type BeyondLimitsHandler struct {
	client      *ent.Client
	settingRepo service.SettingRepository
}

// NewBeyondLimitsHandler 构造破甲管道后台 handler。
func NewBeyondLimitsHandler(client *ent.Client, settingRepo service.SettingRepository) *BeyondLimitsHandler {
	return &BeyondLimitsHandler{client: client, settingRepo: settingRepo}
}

// parseBeyondLimitsPlatform 从 URL :platform 参数解析平台；未知值回退 openai。
func parseBeyondLimitsPlatform(c *gin.Context) service.BeyondLimitsPlatform {
	p := service.BeyondLimitsPlatform(strings.TrimSpace(c.Param("platform")))
	switch p {
	case service.BeyondLimitsPlatformDeepSeek, service.BeyondLimitsPlatformGemini:
		return p
	default:
		return service.BeyondLimitsPlatformOpenAI
	}
}

// GetConfig 返回当前平台管道配置与内置默认值。
func (h *BeyondLimitsHandler) GetConfig(c *gin.Context) {
	platform := parseBeyondLimitsPlatform(c)
	cfg, engine := service.GetBeyondLimitsConfigFor(platform)
	rules := cfg.TamperRules
	if len(rules) == 0 {
		rules = service.DefaultBeyondLimitsTamperRules()
	}
	response.Success(c, gin.H{
		"platform":             string(platform),
		"bridge":               cfg.Bridge,
		"tamper_rules":         rules,
		"rule_count":           engine.RuleCount(),
		"default_bridge":       cfg.DefaultBridge,
		"default_tamper_rules": service.DefaultBeyondLimitsTamperRules(),
	})
}

type beyondLimitsConfigRequest struct {
	Bridge      *string   `json:"bridge"`
	TamperRules *[]string `json:"tamper_rules"`
}

// UpdateConfig 更新当前平台管道配置并同步运行时。
func (h *BeyondLimitsHandler) UpdateConfig(c *gin.Context) {
	platform := parseBeyondLimitsPlatform(c)
	var req beyondLimitsConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	cfg, _ := service.GetBeyondLimitsConfigFor(platform)
	if req.Bridge != nil {
		cfg.Bridge = *req.Bridge
	}
	if req.TamperRules != nil {
		cfg.TamperRules = *req.TamperRules
	}
	if err := service.SaveBeyondLimitsConfig(c.Request.Context(), h.settingRepo, platform, cfg); err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to save config: "+err.Error())
		return
	}
	h.GetConfig(c)
}

type beyondLimitsToggleGroupRequest struct {
	Enabled bool `json:"enabled"`
}

// ToggleGroup 开启/关闭某个分组在当前平台下的破甲管道。
func (h *BeyondLimitsHandler) ToggleGroup(c *gin.Context) {
	platform := parseBeyondLimitsPlatform(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid group id")
		return
	}
	var req beyondLimitsToggleGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	upd := h.client.Group.UpdateOneID(id)
	field := "beyond_limits_enabled"
	switch platform {
	case service.BeyondLimitsPlatformDeepSeek:
		upd.SetBeyondLimitsDeepseekEnabled(req.Enabled)
		field = "beyond_limits_deepseek_enabled"
	case service.BeyondLimitsPlatformGemini:
		upd.SetBeyondLimitsGeminiEnabled(req.Enabled)
		field = "beyond_limits_gemini_enabled"
	default:
		upd.SetBeyondLimitsEnabled(req.Enabled)
	}
	if _, err := upd.Save(c.Request.Context()); err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to update group: "+err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "platform": string(platform), field: req.Enabled})
}

// Stats 返回当前平台破甲交互聚合统计。
func (h *BeyondLimitsHandler) Stats(c *gin.Context) {
	platform := parseBeyondLimitsPlatform(c)
	response.Success(c, service.BeyondLimitsStatsSnapshot(platform))
}

// Log 返回当前平台最近破甲交互日志。
func (h *BeyondLimitsHandler) Log(c *gin.Context) {
	platform := parseBeyondLimitsPlatform(c)
	response.Success(c, service.BeyondLimitsLogSnapshot(platform))
}
