package repository

import (
	"errors"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
	"gorm.io/gorm"
)

// GetPlatformLink 查询某个用户在本应用下的平台绑定。
func GetPlatformLink(userID string) (model.PlatformLink, bool, error) {
	db, err := DB()
	if err != nil {
		return model.PlatformLink{}, false, err
	}
	link := model.PlatformLink{}
	err = db.Where("user_id = ?", strings.TrimSpace(userID)).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.PlatformLink{}, false, nil
	}
	return link, err == nil, err
}

// GetPlatformLinkByPlatformUser 查询某个平台用户在本应用下的绑定。
func GetPlatformLinkByPlatformUser(platformUser int) (model.PlatformLink, bool, error) {
	db, err := DB()
	if err != nil {
		return model.PlatformLink{}, false, err
	}
	link := model.PlatformLink{}
	err = db.Where("platform_user = ?", platformUser).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.PlatformLink{}, false, nil
	}
	return link, err == nil, err
}

// SavePlatformLink 写入或更新平台绑定。
func SavePlatformLink(link model.PlatformLink) (model.PlatformLink, error) {
	db, err := DB()
	if err != nil {
		return link, err
	}
	return link, db.Save(&link).Error
}

// DeletePlatformLink 解除某个用户在本应用下的平台绑定。
func DeletePlatformLink(userID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.PlatformLink{}, "user_id = ?", strings.TrimSpace(userID)).Error
}
