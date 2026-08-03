package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/httpx"
)

func main() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/api/health", func(c *gin.Context) { httpx.OK(c, http.StatusOK, gin.H{"status": "ok"}) })
	log.Println("listening on :8080")
	log.Fatal(r.Run(":8080"))
}
