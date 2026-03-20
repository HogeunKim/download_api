package config

import "path/filepath"

// GetPath는 OS에 관계없이 올바른 경로 구분자를 사용하여 경로를 병합합니다.
func GetPath(paths ...string) string {
	return filepath.Join(paths...)
}
