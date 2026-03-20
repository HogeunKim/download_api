//go:build linux

package handler

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
)

var windowsDrivePrefixPattern = regexp.MustCompile(`^[a-zA-Z]:[\\/].*`)

func validateTargetFolderByOS(targetFolder string) error {
	trimmed := strings.TrimSpace(targetFolder)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, `\`) || windowsDrivePrefixPattern.MatchString(trimmed) || strings.HasPrefix(trimmed, `\\`) {
		return errors.New("targetFolder must use linux path format when running on linux")
	}
	if !filepath.IsAbs(trimmed) {
		return errors.New("targetFolder must be an absolute linux path when running on linux")
	}
	return nil
}
