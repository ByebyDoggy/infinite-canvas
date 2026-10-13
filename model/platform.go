package model

// PlatformLink 记录一个 New API 用户与本应用账号的绑定。
// APIKey 是该用户在本应用下的专用 sk- 密钥，只在服务端使用，绝不返回给前端。
type PlatformLink struct {
	UserID        string `json:"userId" gorm:"primaryKey"`      // 本应用用户 ID
	PlatformUser  int    `json:"platformUser" gorm:"index"`     // New API user_id
	Username      string `json:"username"`                      // New API 用户名，仅展示
	Language      string `json:"language"`                      // 面板语言偏好
	APIKey        string `json:"-" gorm:"type:text"`            // sk-…，永不序列化
	APIBaseURL    string `json:"apiBaseUrl"`                    // 平台 relay 根地址，不含 /v1
	AuthorizeURL  string `json:"authorizeUrl"`                  // 平台托管授权页
	UnlimitedQuot bool   `json:"unlimitedQuota"`
	Quota         int64  `json:"quota"`
	UsedQuota     int64  `json:"usedQuota"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}
