package service

import (
	"errors"
	"sort"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

const (
	OrderStatusPendingReceipt = "PENDING_RECEIPT"
	OrderStatusPendingReview  = "PENDING_REVIEW"
	OrderStatusApproved       = "APPROVED"
	OrderStatusRejected       = "REJECTED"
)

// ShopInboundOption holds inbound info with shop availability.
type ShopInboundOption struct {
	Id       int    `json:"id"`
	Remark   string `json:"remark"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
	Enabled  bool   `json:"enabled"`
}

// ShopService provides operations for packages and orders.
type ShopService struct {
	inboundService InboundService
	settingService SettingService
}

func (s *ShopService) ListPackages(activeOnly bool) ([]model.ShopPackage, error) {
	db := database.GetDB()
	var packages []model.ShopPackage
	query := db.Model(&model.ShopPackage{})
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	err := query.Order("id desc").Find(&packages).Error
	return packages, err
}

func (s *ShopService) CreatePackage(pkg *model.ShopPackage) error {
	pkg.CreatedAt = time.Now()
	pkg.UpdatedAt = time.Now()
	return database.GetDB().Create(pkg).Error
}

func (s *ShopService) UpdatePackage(pkg *model.ShopPackage) error {
	pkg.UpdatedAt = time.Now()
	return database.GetDB().Model(&model.ShopPackage{}).Where("id = ?", pkg.Id).Updates(pkg).Error
}

func (s *ShopService) DeletePackage(id int) error {
	return database.GetDB().Delete(&model.ShopPackage{}, id).Error
}

func (s *ShopService) GetPackage(id int) (*model.ShopPackage, error) {
	db := database.GetDB()
	pkg := &model.ShopPackage{}
	if err := db.First(pkg, id).Error; err != nil {
		return nil, err
	}
	return pkg, nil
}

func (s *ShopService) ListOrders() ([]model.ShopOrder, error) {
	db := database.GetDB()
	var orders []model.ShopOrder
	err := db.Order("id desc").Find(&orders).Error
	return orders, err
}

func (s *ShopService) ListOrdersByTelegramId(tgId int64) ([]model.ShopOrder, error) {
	db := database.GetDB()
	var orders []model.ShopOrder
	err := db.Where("telegram_id = ?", tgId).Order("id desc").Find(&orders).Error
	return orders, err
}

func (s *ShopService) GetOrder(id int) (*model.ShopOrder, error) {
	db := database.GetDB()
	order := &model.ShopOrder{}
	if err := db.First(order, id).Error; err != nil {
		return nil, err
	}
	return order, nil
}

func (s *ShopService) CreateOrder(order *model.ShopOrder) error {
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()
	return database.GetDB().Create(order).Error
}

func (s *ShopService) UpdateOrder(order *model.ShopOrder) error {
	order.UpdatedAt = time.Now()
	return database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", order.Id).Updates(order).Error
}

func (s *ShopService) UpdateOrderReceipt(id int, receiptPath, receiptFileId string) error {
	return database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", id).Updates(map[string]any{
		"receipt_path":    receiptPath,
		"receipt_file_id": receiptFileId,
		"status":          OrderStatusPendingReview,
		"updated_at":      time.Now(),
	}).Error
}

func (s *ShopService) UpdateOrderStatus(id int, status, note string) error {
	return database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	}).Error
}

func (s *ShopService) SetOrderProvisioned(id int, email, clientId, subId string) error {
	return database.GetDB().Model(&model.ShopOrder{}).Where("id = ?", id).Updates(map[string]any{
		"client_email":  email,
		"client_id":     clientId,
		"client_sub_id": subId,
		"status":        OrderStatusApproved,
		"updated_at":    time.Now(),
	}).Error
}

func (s *ShopService) ListInbounds() ([]ShopInboundOption, error) {
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}

	db := database.GetDB()
	var shopInbounds []model.ShopInbound
	_ = db.Find(&shopInbounds).Error
	enabledMap := map[int]bool{}
	for _, item := range shopInbounds {
		enabledMap[item.InboundId] = item.Enabled
	}

	// If none configured, default to all enabled.
	useDefaultAll := len(shopInbounds) == 0

	options := make([]ShopInboundOption, 0, len(inbounds))
	for _, inbound := range inbounds {
		enabled := enabledMap[inbound.Id]
		if useDefaultAll {
			enabled = true
		}
		options = append(options, ShopInboundOption{
			Id:       inbound.Id,
			Remark:   inbound.Remark,
			Protocol: string(inbound.Protocol),
			Port:     inbound.Port,
			Enabled:  enabled,
		})
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].Id < options[j].Id
	})

	return options, nil
}

func (s *ShopService) SetInboundEnabled(inboundId int, enabled bool) error {
	db := database.GetDB()
	var existing model.ShopInbound
	err := db.Where("inbound_id = ?", inboundId).First(&existing).Error
	if err == nil {
		return db.Model(&model.ShopInbound{}).Where("id = ?", existing.Id).Updates(map[string]any{
			"enabled":    enabled,
			"updated_at": time.Now(),
		}).Error
	}

	return db.Create(&model.ShopInbound{
		InboundId: inboundId,
		Enabled:   enabled,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error
}

func (s *ShopService) EnabledInboundIds() ([]int, error) {
	inbounds, err := s.ListInbounds()
	if err != nil {
		return nil, err
	}
	var ids []int
	for _, ib := range inbounds {
		if ib.Enabled {
			ids = append(ids, ib.Id)
		}
	}
	return ids, nil
}

func (s *ShopService) ValidateCustomOrder(dataGB, days int) error {
	minGb, _ := s.settingService.GetShopMinGB()
	maxGb, _ := s.settingService.GetShopMaxGB()
	minDays, _ := s.settingService.GetShopMinDays()
	maxDays, _ := s.settingService.GetShopMaxDays()

	if minGb > 0 && dataGB < minGb {
		return errors.New("data less than minimum allowed")
	}
	if maxGb > 0 && dataGB > maxGb {
		return errors.New("data greater than maximum allowed")
	}
	if minDays > 0 && days < minDays {
		return errors.New("days less than minimum allowed")
	}
	if maxDays > 0 && days > maxDays {
		return errors.New("days greater than maximum allowed")
	}
	return nil
}

func (s *ShopService) CalculateCustomPrice(dataGB int) (int64, error) {
	pricePerGb, err := s.settingService.GetShopPricePerGB()
	if err != nil {
		return 0, err
	}
	if pricePerGb < 0 {
		pricePerGb = 0
	}
	return int64(dataGB * pricePerGb), nil
}
