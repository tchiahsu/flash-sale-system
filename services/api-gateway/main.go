package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

type OrderRequest struct {
	EventID  string `json:"event_id"`
	UserID   string `json:"user_id"`
	Quantity int    `json:"quantity"`
}

var (
	orderServiceURL string
	httpClient      = &http.Client{Timeout: 5 * time.Second}
)

func init() {
	orderServiceURL = os.Getenv("ORDER_SERVICE_URL")
	if orderServiceURL == "" {
		orderServiceURL = "http://localhost:8080"
	}
}

func handleOrders(c *gin.Context) {
	var request OrderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	body, _ := json.Marshal(request)
	response, err := httpClient.Post(orderServiceURL+"/api/process-order", "application/json", bytes.NewBuffer(body))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "order service unavailable"})
		return
	}
	defer response.Body.Close()

	resBody, _ := io.ReadAll(response.Body)
	c.Data(response.StatusCode, "application/json", resBody)
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

func main() {
	app := gin.Default()
	app.POST("/api/orders", handleOrders)
	app.GET("/health", healthCheck)
	app.Run(":8080")
}
