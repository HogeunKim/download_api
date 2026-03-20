//go:build windows

package config

// GetDefaultLogDir는 Windows 환경의 로그 저장 경로를 반환합니다.
func GetDefaultLogDir() string {
	return "C:\\temp\\myapp\\logs"
}
