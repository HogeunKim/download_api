package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"go-api-server/internal/config"
	"go-api-server/internal/handler"
	"go-api-server/internal/service"
)

func main() {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service.StartHostScanScheduler(rootCtx)

	// Go 1.22+의 새로운 라우팅 패턴 사용
	mux := http.NewServeMux()

	// 라우팅 설정
	mux.HandleFunc("GET /users", handler.UserHandler)
	mux.HandleFunc("POST /users", handler.UserHandler)
	mux.HandleFunc("POST /info", handler.InfoHandler)
	mux.HandleFunc("POST /download", handler.DownloadHandler)
	mux.HandleFunc("GET /config", handler.CgiConfigHandler)
	mux.HandleFunc("PUT /config", handler.CgiConfigHandler)
	mux.HandleFunc("GET /host-scan/config", handler.HostScanConfigHandler)
	mux.HandleFunc("PUT /host-scan/config", handler.HostScanConfigHandler)
	mux.HandleFunc("GET /host-scan/scheduler", handler.HostScanSchedulerHandler)
	mux.HandleFunc("PUT /host-scan/scheduler", handler.HostScanSchedulerHandler)

	// OS별 로그 경로 확인 (빌드 태그 확인용)
	logPath := config.GetDefaultLogDir()
	port := ":9800"

	fmt.Println("========================================")
	fmt.Printf("Go API Server 가동 중...\n")
	fmt.Printf("접속 주소: http://localhost%s\n", port)
	fmt.Printf("현재 OS 로그 경로: %s\n", logPath)
	fmt.Println("========================================")

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("서버 실행 실패: %v", err)
	}
}
