package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type OrderRequest struct {
	EventID  string `json:"event_id"`
	UserID   string `json:"user_id"`
	Quantity int    `json:"quantity"`
}

type ResetResult struct {
	Service string `json:"service"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

var (
	orderServiceURL        string
	inventoryServiceURL    string
	notificationServiceURL string
	httpClient             = &http.Client{Timeout: 5 * time.Second}
)

func init() {
	orderServiceURL = os.Getenv("ORDER_SERVICE_URL")
	inventoryServiceURL = os.Getenv("INVENTORY_SERVICE_URL")
	notificationServiceURL = os.Getenv("NOTIFICATION_SERVICE_URL")
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

func handleReset(c *gin.Context) {
	services := []struct {
		name string
		url  string
	}{
		{"inventory", inventoryServiceURL + "/api/reset"},
		{"order", orderServiceURL + "/api/reset"},
		{"notification", notificationServiceURL + "/api/reset"},
	}

	results := make([]ResetResult, len(services))
	var wg sync.WaitGroup

	for i, svc := range services {
		wg.Add(1)
		go func(i int, name, url string) {
			defer wg.Done()
			results[i] = callReset(name, url)
		}(i, svc.name, svc.url)
	}

	wg.Wait()

	allSuccess := true
	for _, r := range results {
		if !r.Success {
			allSuccess = false
			break
		}
	}

	status := http.StatusOK
	if !allSuccess {
		status = http.StatusInternalServerError
	}

	c.JSON(status, gin.H{
		"success": allSuccess,
		"results": results,
	})
}

func callReset(service, url string) ResetResult {
	resp, err := httpClient.Post(url, "application/json", nil)
	if err != nil {
		return ResetResult{Service: service, Success: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	return ResetResult{
		Service: service,
		Success: resp.StatusCode == http.StatusOK,
	}
}

func healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

func main() {
	app := gin.Default()
	app.POST("/api/orders", handleOrders)
	app.POST("/api/reset", handleReset)
	app.GET("/health", healthCheck)
	app.Run(":8080")
}
