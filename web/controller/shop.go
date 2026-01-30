package controller

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/web/service"

	"github.com/gin-gonic/gin"
)

// ShopController handles package/order management.
type ShopController struct {
	BaseController
	shopService   service.ShopService
	tgbotService  service.Tgbot
}

// NewShopController creates a ShopController instance.
func NewShopController(g *gin.RouterGroup) *ShopController {
	s := &ShopController{}
	s.initRouter(g)
	return s
}

func (s *ShopController) initRouter(g *gin.RouterGroup) {
	shop := g.Group("/shop")

	shop.GET("/packages", s.listPackages)
	shop.POST("/packages", s.upsertPackage)
	shop.POST("/packages/:id/delete", s.deletePackage)

	shop.GET("/orders", s.listOrders)
	shop.POST("/orders/:id/approve", s.approveOrder)
	shop.POST("/orders/:id/reject", s.rejectOrder)
	shop.GET("/receipt/:id", s.getReceipt)

	shop.GET("/inbounds", s.listInbounds)
	shop.POST("/inbounds/:id", s.setInboundEnabled)
}

func (s *ShopController) listPackages(c *gin.Context) {
	packages, err := s.shopService.ListPackages(false)
	jsonObj(c, packages, err)
}

func (s *ShopController) upsertPackage(c *gin.Context) {
	pkg := &model.ShopPackage{}
	if err := c.ShouldBindJSON(pkg); err != nil {
		jsonMsg(c, "invalid package", err)
		return
	}
	if pkg.Name == "" {
		jsonMsg(c, "name is required", nil)
		return
	}
	if pkg.Id > 0 {
		err := s.shopService.UpdatePackage(pkg)
		jsonMsg(c, "updated", err)
		return
	}
	err := s.shopService.CreatePackage(pkg)
	jsonMsg(c, "created", err)
}

func (s *ShopController) deletePackage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "invalid id", err)
		return
	}
	err = s.shopService.DeletePackage(id)
	jsonMsg(c, "deleted", err)
}

func (s *ShopController) listOrders(c *gin.Context) {
	orders, err := s.shopService.ListOrders()
	if err != nil {
		jsonMsg(c, "failed to get orders", err)
		return
	}
	packages, _ := s.shopService.ListPackages(false)
	resp := gin.H{
		"orders":   orders,
		"packages": packages,
	}
	jsonObj(c, resp, nil)
}

func (s *ShopController) approveOrder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "invalid id", err)
		return
	}
	order, err := s.shopService.GetOrder(id)
	if err != nil {
		jsonMsg(c, "order not found", err)
		return
	}

	if order.Status != service.OrderStatusPendingReview {
		jsonMsg(c, "order not ready", nil)
		return
	}

	email, clientId, subId, err := s.tgbotService.ProvisionOrder(order)
	if err != nil {
		jsonMsg(c, "provision failed", err)
		return
	}

	if err := s.shopService.SetOrderProvisioned(order.Id, email, clientId, subId); err != nil {
		logger.Warning("order provision saved partially:", err)
	}

	// notify user if bot is running
	s.tgbotService.SendOrderFulfillment(order.TelegramId, email)
	jsonMsg(c, "approved", nil)
}

func (s *ShopController) rejectOrder(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "invalid id", err)
		return
	}
	err = s.shopService.UpdateOrderStatus(id, service.OrderStatusRejected, "")
	jsonMsg(c, "rejected", err)
}

func (s *ShopController) listInbounds(c *gin.Context) {
	inbounds, err := s.shopService.ListInbounds()
	jsonObj(c, inbounds, err)
}

func (s *ShopController) setInboundEnabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "invalid id", err)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, "invalid request", err)
		return
	}
	err = s.shopService.SetInboundEnabled(id, body.Enabled)
	jsonMsg(c, "updated", err)
}

func (s *ShopController) getReceipt(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	order, err := s.shopService.GetOrder(id)
	if err != nil || order.ReceiptPath == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	path := filepath.Clean(order.ReceiptPath)
	if _, err := os.Stat(path); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.File(path)
}
