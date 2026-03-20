package service

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go-api-server/internal/model"
)

type TargetHTTPError struct {
	StatusCode int
	Message    string
}

func (e *TargetHTTPError) Error() string {
	return fmt.Sprintf("target returned status %d: %s", e.StatusCode, e.Message)
}

func FetchInfoFromTarget(ctx context.Context, req model.InfoRequest) (map[string]any, error) {
	cfg := GetHostScanCGIConfig()
	return FetchInfoFromTargetWithCredentials(ctx, req.IP, cfg.Port, cfg.User, cfg.PW)
}

func FetchInfoFromTargetWithCredentials(ctx context.Context, ip string, port int, user, pw string) (map[string]any, error) {
	targetURL := fmt.Sprintf("http://%s/info.cgi", net.JoinHostPort(ip, fmt.Sprintf("%d", port)))
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	body, statusCode, err := doDigestRequest(ctx, client, http.MethodGet, targetURL, user, pw)
	if err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = "empty error body"
		}
		return nil, &TargetHTTPError{
			StatusCode: statusCode,
			Message:    msg,
		}
	}

	return ParseInfoCGIResponse(body), nil
}

func ParseInfoCGIResponse(body []byte) map[string]any {
	result := make(map[string]any)
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	inPrivilege := false
	privileges := make([]string, 0)

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		switch line {
		case "[Privilege]":
			inPrivilege = true
			continue
		case "[/Privilege]":
			inPrivilege = false
			result["Privilege"] = privileges
			continue
		}

		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") {
			continue
		}

		if inPrivilege {
			privileges = append(privileges, line)
			continue
		}

		key, value, ok := splitKeyValueLine(line)
		if !ok {
			continue
		}
		result[key] = value
	}

	return result
}

func splitKeyValueLine(line string) (string, string, bool) {
	if line == "" {
		return "", "", false
	}

	if idx := strings.Index(line, ":"); idx > 0 {
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key != "" {
			return key, value, true
		}
	}

	if idx := strings.Index(line, "="); idx > 0 {
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		if key != "" {
			return key, value, true
		}
	}

	return "", "", false
}

type digestChallenge struct {
	Realm     string
	Nonce     string
	Opaque    string
	Algorithm string
	QOP       string
}

func doDigestRequest(ctx context.Context, client *http.Client, method, url, username, password string) ([]byte, int, error) {
	firstReq, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	firstResp, err := client.Do(firstReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to request target: %w", err)
	}
	defer firstResp.Body.Close()

	if firstResp.StatusCode != http.StatusUnauthorized {
		body, readErr := io.ReadAll(firstResp.Body)
		if readErr != nil {
			return nil, firstResp.StatusCode, fmt.Errorf("failed to read target response: %w", readErr)
		}
		return body, firstResp.StatusCode, nil
	}

	challengeHeader := firstResp.Header.Get("WWW-Authenticate")
	challenge, err := parseDigestChallenge(challengeHeader)
	if err != nil {
		return nil, 0, err
	}

	authHeader, err := buildDigestAuthHeader(method, url, username, password, challenge)
	if err != nil {
		return nil, 0, err
	}

	secondReq, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create authenticated request: %w", err)
	}
	secondReq.Header.Set("Authorization", authHeader)

	secondResp, err := client.Do(secondReq)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to request target with digest auth: %w", err)
	}
	defer secondResp.Body.Close()

	body, err := io.ReadAll(secondResp.Body)
	if err != nil {
		return nil, secondResp.StatusCode, fmt.Errorf("failed to read target response: %w", err)
	}

	return body, secondResp.StatusCode, nil
}

func parseDigestChallenge(header string) (digestChallenge, error) {
	const prefix = "Digest "
	if !strings.HasPrefix(header, prefix) {
		return digestChallenge{}, fmt.Errorf("target does not provide digest auth challenge")
	}

	params := parseChallengeParams(strings.TrimSpace(strings.TrimPrefix(header, prefix)))
	if params["realm"] == "" || params["nonce"] == "" {
		return digestChallenge{}, fmt.Errorf("invalid digest challenge from target")
	}

	algorithm := params["algorithm"]
	if algorithm == "" {
		algorithm = "MD5"
	}

	return digestChallenge{
		Realm:     params["realm"],
		Nonce:     params["nonce"],
		Opaque:    params["opaque"],
		Algorithm: algorithm,
		QOP:       params["qop"],
	}, nil
}

func parseChallengeParams(raw string) map[string]string {
	result := make(map[string]string)
	parts := splitIgnoringQuotedComma(raw)
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.TrimSpace(kv[1])
		val = strings.Trim(val, `"`)
		result[key] = val
	}
	return result
}

func splitIgnoringQuotedComma(s string) []string {
	var parts []string
	start := 0
	inQuotes := false

	for i, r := range s {
		if r == '"' {
			inQuotes = !inQuotes
			continue
		}
		if r == ',' && !inQuotes {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func buildDigestAuthHeader(method, rawURL, username, password string, c digestChallenge) (string, error) {
	parsed, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to parse target url: %w", err)
	}

	uri := parsed.URL.RequestURI()
	nc := "00000001"
	cnonce, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("failed to generate digest cnonce: %w", err)
	}

	qop := selectQOP(c.QOP)
	ha1, err := buildHA1(username, password, c.Realm, c.Nonce, cnonce, c.Algorithm)
	if err != nil {
		return "", err
	}
	ha2, err := hashByAlgorithm(c.Algorithm, fmt.Sprintf("%s:%s", method, uri))
	if err != nil {
		return "", err
	}

	var response string
	if qop == "" {
		response, err = hashByAlgorithm(c.Algorithm, fmt.Sprintf("%s:%s:%s", ha1, c.Nonce, ha2))
		if err != nil {
			return "", err
		}
	} else {
		response, err = hashByAlgorithm(c.Algorithm, fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, c.Nonce, nc, cnonce, qop, ha2))
		if err != nil {
			return "", err
		}
	}

	fields := []string{
		fmt.Sprintf(`username="%s"`, username),
		fmt.Sprintf(`realm="%s"`, c.Realm),
		fmt.Sprintf(`nonce="%s"`, c.Nonce),
		fmt.Sprintf(`uri="%s"`, uri),
		fmt.Sprintf(`response="%s"`, response),
	}
	if c.Algorithm != "" {
		fields = append(fields, fmt.Sprintf("algorithm=%s", c.Algorithm))
	}
	if c.Opaque != "" {
		fields = append(fields, fmt.Sprintf(`opaque="%s"`, c.Opaque))
	}
	if qop != "" {
		fields = append(fields,
			fmt.Sprintf("qop=%s", qop),
			fmt.Sprintf("nc=%s", nc),
			fmt.Sprintf(`cnonce="%s"`, cnonce),
		)
	}

	return "Digest " + strings.Join(fields, ", "), nil
}

func buildHA1(username, password, realm, nonce, cnonce, algorithm string) (string, error) {
	base, err := hashByAlgorithm(algorithm, fmt.Sprintf("%s:%s:%s", username, realm, password))
	if err != nil {
		return "", err
	}

	if strings.EqualFold(algorithm, "MD5-sess") || strings.EqualFold(algorithm, "SHA-256-sess") {
		return hashByAlgorithm(algorithm, fmt.Sprintf("%s:%s:%s", base, nonce, cnonce))
	}
	return base, nil
}

func selectQOP(raw string) string {
	if raw == "" {
		return ""
	}
	for _, candidate := range strings.Split(raw, ",") {
		val := strings.TrimSpace(strings.ToLower(candidate))
		if val == "auth" {
			return "auth"
		}
	}
	first := strings.TrimSpace(strings.Split(raw, ",")[0])
	return strings.ToLower(first)
}

func hashByAlgorithm(algorithm, payload string) (string, error) {
	switch strings.ToUpper(algorithm) {
	case "", "MD5", "MD5-SESS":
		sum := md5.Sum([]byte(payload))
		return hex.EncodeToString(sum[:]), nil
	case "SHA-256", "SHA-256-SESS":
		sum := sha256.Sum256([]byte(payload))
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported digest algorithm: %s", algorithm)
	}
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
