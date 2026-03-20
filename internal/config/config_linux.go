//go:build linux

package config

// GetDefaultLogDir는 Linux 환경의 로그 저장 경로를 반환합니다.
func GetDefaultLogDir() string {
	return "/var/log/myapp"
}
