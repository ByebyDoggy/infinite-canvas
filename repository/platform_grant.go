package repository

import (
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
	"gorm.io/gorm"
)

// ListPlatformGrants 读取某个用户在本应用下的全部平台授权项，按 priority 升序。
func ListPlatformGrants(userID string) ([]model.PlatformGrant, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	grants := []model.PlatformGrant{}
	err = db.Where("user_id = ?", strings.TrimSpace(userID)).
		Order("priority asc, grant_key asc").Find(&grants).Error
	return grants, err
}

// ReplacePlatformGrants 用一次 exchange 的结果整体替换该用户的授权项。
//
// 每次登录都以平台返回为准：平台侧增删 grant 后，本地不能残留指向已撤销密钥的
// 记录，否则路由会把请求打到一张死密钥上。空列表表示该用户已没有任何授权。
func ReplacePlatformGrants(userID string, grants []model.PlatformGrant) error {
	db, err := DB()
	if err != nil {
		return err
	}
	userID = strings.TrimSpace(userID)
	for index := range grants {
		grants[index].UserID = userID
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&model.PlatformGrant{}).Error; err != nil {
			return err
		}
		if len(grants) == 0 {
			return nil
		}
		return tx.Create(&grants).Error
	})
}

// MarkPlatformGrantStatus 标记单条授权项的本地状态（例如某张密钥 401 后降级）。
// 只影响本地路由选择，不会改动平台侧的任何凭据。
func MarkPlatformGrantStatus(userID string, grantKey string, status string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.PlatformGrant{}).
		Where("user_id = ? AND grant_key = ?", strings.TrimSpace(userID), strings.TrimSpace(grantKey)).
		Update("status", strings.TrimSpace(status)).Error
}

// DeletePlatformGrants 删除某个用户的全部授权项（断开连接时使用）。
func DeletePlatformGrants(userID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Where("user_id = ?", strings.TrimSpace(userID)).Delete(&model.PlatformGrant{}).Error
}
