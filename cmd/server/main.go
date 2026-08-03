package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/bootstrap"
	"phum-panya/internal/config"
	"phum-panya/internal/db"
	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		runCreateAdmin()
		return
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/api/health", func(c *gin.Context) { httpx.OK(c, http.StatusOK, gin.H{"status": "ok"}) })
	log.Println("listening on :8080")
	log.Fatal(r.Run(":8080"))
}

// runCreateAdmin seeds the first central admin from APP_ADMIN_EMAIL and
// APP_ADMIN_PASSWORD, then exits.
func runCreateAdmin() {
	cfg := config.Load()
	g, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	created, err := bootstrap.EnsureAdmin(g, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		log.Fatalf("ensure admin: %v", err)
	}
	if created {
		fmt.Println("admin created")
	} else {
		fmt.Println("admin already exists or nothing to seed")
	}
}
