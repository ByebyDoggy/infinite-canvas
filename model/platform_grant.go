package model

// PlatformGrantStatusActive 表示这条授权项当前可用；任何一张密钥收到 401 后
// 只把对应 grant 降级，不影响同用户的其他授权项。
const (
	PlatformGrantStatusActive  = "active"
	PlatformGrantStatusRevoked = "revoked"
)

// PlatformGrantKeyLegacy 是平台 v1 单密钥语义下唯一的授权项标识。
const PlatformGrantKeyLegacy = "legacy"

// PlatformGrant 是一条平台授权项：平台按「(分组, 模型集合)」给用户签发一张
// 专用密钥，应用侧按模型把它路由到对应密钥。
//
// 一条 grant 对应一张 sk-，只在服务端保存，永不返回前端。
// GrantKey 是平台给的稳定标识（auto:<spec> / manual:<group> / legacy）。
type PlatformGrant struct {
	UserID    string   `json:"userId" gorm:"primaryKey"`   // 本应用用户 ID
	GrantKey  string   `json:"grantKey" gorm:"primaryKey"` // 平台下发的 grant.id
	Label     string   `json:"label"`                      // 展示名，可能为空
	Group     string   `json:"group"`                      // 平台分组，仅展示与排障
	Models    []string `json:"models" gorm:"serializer:json"`
	APIKey    string   `json:"-" gorm:"type:text"` // sk-…，永不序列化
	Priority  int      `json:"priority"`           // 越小越优先
	Status    string   `json:"status"`             // active / revoked（本地降级标记）
	CreatedAt string   `json:"createdAt"`
	UpdatedAt string   `json:"updatedAt"`
}
