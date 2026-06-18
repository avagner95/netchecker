package app

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maxAlfaDiskUploadBytes = 60 * 1024 * 1024

type alfaDiskUploadSpec struct {
	Kind      string
	URL       string `json:"url"`
	Method    string `json:"method"`
	BaseURL   string
	LinkToken string
	ItemName  string
	ShareKey  string
	Password  string `json:"password"`
}

type alfaBoxInfoResponse struct {
	Permissions []string `json:"permissions"`
}

type alfaDiskTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type alfaDiskRoomResponse struct {
	Code  int `json:"code"`
	Share struct {
		Entity struct {
			ID string `json:"id"`
		} `json:"entity"`
	} `json:"share"`
	ErrorUID string `json:"errorUid"`
}

type alfaDiskUploadKeyResponse struct {
	Code            int    `json:"code"`
	UploadKey       string `json:"uploadKey"`
	PackageSize     int64  `json:"packageSize"`
	PackageCount    int    `json:"packageCount"`
	CompletedChunks int    `json:"completedChunks"`
	ErrorUID        string `json:"errorUid"`
}

type alfaDiskUploadChunkResponse struct {
	Code     int    `json:"code"`
	Status   int    `json:"status"`
	ErrorUID string `json:"errorUid"`
}

type alfaDiskUploadCompleteResponse struct {
	Code     int    `json:"code"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	ErrorUID string `json:"errorUid"`
}

func (a *App) ExportAndUploadToAlfaDisk(uploadURL string) (string, error) {
	spec, err := parseAlfaDiskUploadSpec(uploadURL)
	if err != nil {
		return "", err
	}

	a.mu.RLock()
	st := a.store
	appDir := a.AppDir
	clientID := a.clientID
	a.mu.RUnlock()

	if st == nil {
		return "", fmt.Errorf("store is nil")
	}

	exportDir := filepath.Join(appDir, "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir export dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.csv.gz", safeFilenamePrefix(clientID), time.Now().Format("20060102_150405"))
	localPath := filepath.Join(exportDir, filename)
	defer func() { _ = os.Remove(localPath) }()

	if _, err := st.ExportMergedCSVGZ(context.Background(), localPath, 0, 0); err != nil {
		return "", err
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat export: %w", err)
	}
	if info.Size() > maxAlfaDiskUploadBytes {
		return "", fmt.Errorf("file is too large for AlfaDisk upload: %.2f MB > 60 MB", float64(info.Size())/(1024*1024))
	}

	if err := uploadFileToAlfaDisk(context.Background(), spec, localPath, info.Size()); err != nil {
		return "", err
	}
	return filename, nil
}

func (a *App) ExportAndUploadToConfiguredAlfaDisk() (string, error) {
	a.mu.RLock()
	settings := a.cfg.AlfaDisk
	a.mu.RUnlock()

	if strings.TrimSpace(settings.SharedLink) == "" {
		return "", fmt.Errorf("AlfaDisk shared link is not configured")
	}
	if strings.TrimSpace(settings.Password) == "" {
		return "", fmt.Errorf("AlfaDisk password is not configured")
	}
	return a.ExportAndUploadToAlfaDisk(settings.SharedLink + " " + settings.Password)
}

func parseAlfaDiskUploadSpec(raw string) (alfaDiskUploadSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return alfaDiskUploadSpec{}, fmt.Errorf("upload URL is empty")
	}

	spec := alfaDiskUploadSpec{URL: raw, Method: http.MethodPut}
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &spec); err != nil {
			return alfaDiskUploadSpec{}, fmt.Errorf("invalid upload link JSON: %w", err)
		}
	} else {
		spec.URL, spec.Password = splitAlfaDiskURLAndPassword(raw)
	}

	spec.URL = strings.TrimSpace(spec.URL)
	spec.Method = strings.ToUpper(strings.TrimSpace(spec.Method))
	if spec.Method == "" {
		spec.Method = http.MethodPut
	}

	if isAlfaBoxURL(spec.URL) {
		return parseAlfaBoxShareURL(spec.URL)
	}
	if isAlfaDiskSharedLinkURL(spec.URL) {
		return parseAlfaDiskSharedLinkURL(spec.URL, spec.Password)
	}
	if isAlfaDiskExternalShareURL(spec.URL) {
		return alfaDiskUploadSpec{}, fmt.Errorf("AlfaDisk external-share links require a logged-in Vaulterix/Keycloak browser session; paste a public AlfaBox /u/... upload folder link or uploadFileLinks.url instead")
	}

	if err := validateUploadURL(spec.URL); err != nil {
		return alfaDiskUploadSpec{}, err
	}
	spec.Kind = "presigned"
	return spec, nil
}

func isAlfaBoxURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Host), "alfabox.alfabank.ru")
}

func splitAlfaDiskURLAndPassword(raw string) (string, string) {
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return raw, ""
	}
	first := fields[0]
	if strings.Contains(first, "alfadisk.alfabank.ru/shared-link/") {
		return first, fields[1]
	}
	return raw, ""
}

func isAlfaDiskSharedLinkURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	parts := splitPath(u.Path)
	return strings.Contains(strings.ToLower(u.Host), "alfadisk.alfabank.ru") && len(parts) == 2 && parts[0] == "shared-link"
}

func isAlfaDiskExternalShareURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(u.Host), "alfadisk.alfabank.ru") && strings.Trim(u.Path, "/") == "external-share"
}

func parseAlfaDiskSharedLinkURL(raw string, password string) (alfaDiskUploadSpec, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaDisk shared-link URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return alfaDiskUploadSpec{}, fmt.Errorf("AlfaDisk shared-link URL must start with http:// or https://")
	}
	parts := splitPath(u.Path)
	if len(parts) != 2 || parts[0] != "shared-link" {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaDisk shared-link URL")
	}
	shareKey, err := url.PathUnescape(parts[1])
	if err != nil {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaDisk share key: %w", err)
	}
	if strings.TrimSpace(password) == "" {
		return alfaDiskUploadSpec{}, fmt.Errorf("AlfaDisk shared-link password is required; paste URL and password separated by a space")
	}
	return alfaDiskUploadSpec{
		Kind:     "alfadisk_shared_link",
		BaseURL:  u.Scheme + "://" + u.Host,
		ShareKey: shareKey,
		Password: strings.TrimSpace(password),
	}, nil
}

func parseAlfaBoxShareURL(raw string) (alfaDiskUploadSpec, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaBox URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return alfaDiskUploadSpec{}, fmt.Errorf("AlfaBox URL must start with http:// or https://")
	}

	var linkToken, itemName string
	parts := splitPath(u.Path)
	if len(parts) >= 3 && parts[0] == "u" {
		linkToken = parts[1]
		itemName = parts[2]
	}

	if linkToken == "" && u.Fragment != "" {
		fragmentParts := splitPath(u.Fragment)
		for i := 0; i+3 < len(fragmentParts); i++ {
			if fragmentParts[i] == "shared" {
				linkToken = fragmentParts[i+2]
				itemName = fragmentParts[i+3]
				break
			}
		}
	}

	var errUnescape error
	linkToken, errUnescape = url.PathUnescape(linkToken)
	if errUnescape != nil {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaBox link token: %w", errUnescape)
	}
	itemName, errUnescape = url.PathUnescape(itemName)
	if errUnescape != nil {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaBox item name: %w", errUnescape)
	}
	if linkToken == "" || itemName == "" {
		return alfaDiskUploadSpec{}, fmt.Errorf("invalid AlfaBox folder link")
	}

	return alfaDiskUploadSpec{
		Kind:      "alfabox",
		BaseURL:   u.Scheme + "://" + u.Host,
		LinkToken: linkToken,
		ItemName:  itemName,
	}, nil
}

func splitPath(path string) []string {
	rawParts := strings.Split(strings.Trim(path, "/"), "/")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func validateUploadURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("upload URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid upload URL: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("upload URL must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("upload URL host is empty")
	}
	if u.Query().Get("X-Amz-Signature") == "" || u.Query().Get("X-Amz-Date") == "" {
		return fmt.Errorf("paste uploadFileLinks.url from Alfa API response, not an AlfaDisk page/share link")
	}
	return nil
}

func uploadFileToAlfaDisk(ctx context.Context, spec alfaDiskUploadSpec, path string, size int64) error {
	if spec.Kind == "alfabox" {
		return uploadFileToAlfaBoxFolder(ctx, spec, path, size)
	}
	if spec.Kind == "alfadisk_shared_link" {
		return uploadFileToAlfaDiskSharedLink(ctx, spec, path, size)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, spec.Method, spec.URL, f)
	if err != nil {
		return fmt.Errorf("create upload request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if len(body) > 0 {
		return fmt.Errorf("upload failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("upload failed: %s", resp.Status)
}

func uploadFileToAlfaBoxFolder(ctx context.Context, spec alfaDiskUploadSpec, path string, size int64) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("create cookie jar: %w", err)
	}
	client := &http.Client{
		Timeout: 5 * time.Minute,
		Jar:     jar,
	}

	if err := checkAlfaBoxUploadAccess(ctx, client, spec); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()

	endpoint, err := url.Parse(spec.BaseURL + "/fss/public/link/public/stream/create")
	if err != nil {
		return fmt.Errorf("create AlfaBox upload URL: %w", err)
	}
	q := endpoint.Query()
	q.Set("path", filepath.Base(path))
	q.Set("size", fmt.Sprintf("%d", size))
	q.Set("mtime", fmt.Sprintf("%d", time.Now().UnixMilli()))
	q.Set("linkToken", spec.LinkToken)
	q.Set("itemName", spec.ItemName)
	q.Set("createParents", "true")
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), f)
	if err != nil {
		return fmt.Errorf("create AlfaBox upload request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-HCPAW-FSS-API-VERSION", "4.6.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("AlfaBox upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if len(body) > 0 {
		return fmt.Errorf("AlfaBox upload failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("AlfaBox upload failed: %s", resp.Status)
}

func checkAlfaBoxUploadAccess(ctx context.Context, client *http.Client, spec alfaDiskUploadSpec) error {
	body := strings.NewReader(fmt.Sprintf(
		`{"linkToken":%q,"itemName":%q,"path":"/"}`,
		spec.LinkToken,
		spec.ItemName,
	))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.BaseURL+"/fss/public/link/public/info", body)
	if err != nil {
		return fmt.Errorf("create AlfaBox info request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HCPAW-FSS-API-VERSION", "4.6.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("AlfaBox info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if len(body) > 0 {
			return fmt.Errorf("AlfaBox info failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("AlfaBox info failed: %s", resp.Status)
	}

	var info alfaBoxInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode AlfaBox info: %w", err)
	}
	for _, permission := range info.Permissions {
		if permission == "UPLOAD" {
			return nil
		}
	}
	return fmt.Errorf("AlfaBox folder link does not allow uploads")
}

func uploadFileToAlfaDiskSharedLink(ctx context.Context, spec alfaDiskUploadSpec, path string, size int64) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("create cookie jar: %w", err)
	}
	client := &http.Client{
		Timeout: 5 * time.Minute,
		Jar:     jar,
	}

	email, err := getAlfaDiskAnonymousEmail(ctx, client, spec)
	if err != nil {
		return err
	}
	token, err := getAlfaDiskAnonymousToken(ctx, client, spec.BaseURL, email)
	if err != nil {
		return err
	}
	folderID, err := openAlfaDiskSharedRoom(ctx, client, spec, token)
	if err != nil {
		return err
	}
	if err := uploadFileWithAlfaDiskAPI(ctx, client, spec, token, folderID, path, size); err != nil {
		return err
	}
	return nil
}

func getAlfaDiskAnonymousEmail(ctx context.Context, client *http.Client, spec alfaDiskUploadSpec) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spec.BaseURL+"/api/shares/add/params/"+url.PathEscape(spec.ShareKey), nil)
	if err != nil {
		return "", fmt.Errorf("create AlfaDisk anonymous email request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AlfaDisk anonymous email request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AlfaDisk anonymous email failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	email := strings.TrimSpace(string(body))
	if email == "" || !strings.Contains(email, "@") {
		return "", fmt.Errorf("AlfaDisk anonymous email response is invalid: %s", email)
	}
	return email, nil
}

func getAlfaDiskAnonymousToken(ctx context.Context, client *http.Client, baseURL string, email string) (string, error) {
	verifier, err := randomPKCEVerifier()
	if err != nil {
		return "", err
	}
	challenge := pkceChallenge(verifier)
	state, err := randomPKCEVerifier()
	if err != nil {
		return "", err
	}
	nonce, err := randomPKCEVerifier()
	if err != nil {
		return "", err
	}

	authURL, err := url.Parse(baseURL + "/auth/realms/vltAnonymousRealm/protocol/openid-connect/auth")
	if err != nil {
		return "", err
	}
	q := authURL.Query()
	q.Set("client_id", "vlt-frontend-spa")
	q.Set("redirect_uri", baseURL+"/external-share")
	q.Set("state", state)
	q.Set("response_mode", "fragment")
	q.Set("response_type", "code")
	q.Set("scope", "openid")
	q.Set("nonce", nonce)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("email", email)
	authURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create AlfaDisk auth request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AlfaDisk auth request: %w", err)
	}
	defer resp.Body.Close()
	htmlBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AlfaDisk auth failed: %s", resp.Status)
	}
	action, err := parseHTMLFormAction(string(htmlBody))
	if err != nil {
		return "", err
	}
	actionURL, err := url.Parse(action)
	if err != nil {
		return "", fmt.Errorf("parse AlfaDisk auth form action: %w", err)
	}
	if !actionURL.IsAbs() {
		base, _ := url.Parse(baseURL)
		actionURL = base.ResolveReference(actionURL)
	}

	noRedirectClient := *client
	noRedirectClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	form := url.Values{}
	form.Set("username", email)
	form.Set("login", "")
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, actionURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create AlfaDisk authenticate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AlfaDisk authenticate request: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		return "", fmt.Errorf("AlfaDisk authenticate failed: %s", resp.Status)
	}
	location := resp.Header.Get("Location")
	code, err := codeFromRedirectLocation(location)
	if err != nil {
		return "", err
	}

	tokenForm := url.Values{}
	tokenForm.Set("code", code)
	tokenForm.Set("grant_type", "authorization_code")
	tokenForm.Set("client_id", "vlt-frontend-spa")
	tokenForm.Set("redirect_uri", baseURL+"/external-share")
	tokenForm.Set("code_verifier", verifier)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/auth/realms/vltAnonymousRealm/protocol/openid-connect/token", strings.NewReader(tokenForm.Encode()))
	if err != nil {
		return "", fmt.Errorf("create AlfaDisk token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AlfaDisk token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("AlfaDisk token failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tokenResp alfaDiskTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode AlfaDisk token: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("AlfaDisk token response is empty")
	}
	return tokenResp.AccessToken, nil
}

func openAlfaDiskSharedRoom(ctx context.Context, client *http.Client, spec alfaDiskUploadSpec, token string) (string, error) {
	data := strings.NewReader(fmt.Sprintf(`{"password":%q,"isAgree":true}`, spec.Password))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.BaseURL+"/api/shares/room/"+url.PathEscape(spec.ShareKey), data)
	if err != nil {
		return "", fmt.Errorf("create AlfaDisk room request: %w", err)
	}
	setAlfaDiskAuthHeaders(req, token, "")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AlfaDisk room request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("AlfaDisk room failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var room alfaDiskRoomResponse
	if err := json.NewDecoder(resp.Body).Decode(&room); err != nil {
		return "", fmt.Errorf("decode AlfaDisk room: %w", err)
	}
	if room.ErrorUID != "" || room.Code != 0 {
		return "", fmt.Errorf("AlfaDisk room failed: %s", room.ErrorUID)
	}
	if room.Share.Entity.ID == "" {
		return "", fmt.Errorf("AlfaDisk room response does not include folder id")
	}
	return room.Share.Entity.ID, nil
}

func uploadFileWithAlfaDiskAPI(ctx context.Context, client *http.Client, spec alfaDiskUploadSpec, token string, folderID string, path string, size int64) error {
	filename := filepath.Base(path)
	passMD5 := alfaDiskPasswordMD5(spec.Password)

	keyPayload := map[string]interface{}{
		"folderId":   folderID,
		"fileName":   filename,
		"fileSize":   size,
		"fileType":   "application/gzip",
		"toRewrite":  false,
		"newVersion": false,
	}
	keyBody, err := json.Marshal(keyPayload)
	if err != nil {
		return fmt.Errorf("encode AlfaDisk upload-key: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.BaseURL+"/api/v1/file/upload-key", bytes.NewReader(keyBody))
	if err != nil {
		return fmt.Errorf("create AlfaDisk upload-key request: %w", err)
	}
	setAlfaDiskAuthHeaders(req, token, passMD5)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("AlfaDisk upload-key request: %w", err)
	}
	var uploadKey alfaDiskUploadKeyResponse
	if err := decodeAlfaDiskJSONResponse(resp, &uploadKey); err != nil {
		return err
	}
	if uploadKey.ErrorUID != "" || uploadKey.Code != 0 || uploadKey.UploadKey == "" {
		return fmt.Errorf("AlfaDisk upload-key failed: %s", uploadKey.ErrorUID)
	}
	if uploadKey.PackageSize <= 0 {
		uploadKey.PackageSize = size
	}
	if uploadKey.PackageCount <= 0 {
		uploadKey.PackageCount = 1
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open export: %w", err)
	}
	defer f.Close()

	startPackage := uploadKey.CompletedChunks + 1
	currentByte := int64(uploadKey.CompletedChunks) * uploadKey.PackageSize
	if _, err := f.Seek(currentByte, io.SeekStart); err != nil {
		return fmt.Errorf("seek export: %w", err)
	}
	for packageNumber := startPackage; packageNumber <= uploadKey.PackageCount; packageNumber++ {
		toByte := currentByte + uploadKey.PackageSize
		if packageNumber == uploadKey.PackageCount || toByte > size {
			toByte = size
		}
		if err := uploadAlfaDiskChunk(ctx, client, spec, token, passMD5, f, uploadKey.UploadKey, packageNumber, currentByte, toByte); err != nil {
			return err
		}
		currentByte = toByte
	}

	completeURL := spec.BaseURL + "/api/v1/file/upload-complete?uploadKey=" + url.QueryEscape(uploadKey.UploadKey)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, completeURL, nil)
	if err != nil {
		return fmt.Errorf("create AlfaDisk upload-complete request: %w", err)
	}
	setAlfaDiskAuthHeaders(req, token, passMD5)
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("AlfaDisk upload-complete request: %w", err)
	}
	var complete alfaDiskUploadCompleteResponse
	if err := decodeAlfaDiskJSONResponse(resp, &complete); err != nil {
		return err
	}
	if complete.ErrorUID != "" || complete.Code != 0 {
		return fmt.Errorf("AlfaDisk upload-complete failed: %s", complete.ErrorUID)
	}
	return nil
}

func uploadAlfaDiskChunk(ctx context.Context, client *http.Client, spec alfaDiskUploadSpec, token string, passMD5 string, f *os.File, uploadKey string, packageNumber int, fromByte int64, toByte int64) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("part", filepath.Base(f.Name()))
	if err != nil {
		return fmt.Errorf("create AlfaDisk chunk part: %w", err)
	}
	if _, err := io.CopyN(part, f, toByte-fromByte); err != nil {
		return fmt.Errorf("read AlfaDisk chunk: %w", err)
	}
	_ = writer.WriteField("uploadKey", uploadKey)
	_ = writer.WriteField("packageNumber", fmt.Sprintf("%d", packageNumber))
	_ = writer.WriteField("fromByte", fmt.Sprintf("%d", fromByte))
	_ = writer.WriteField("toByte", fmt.Sprintf("%d", toByte))
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close AlfaDisk multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.BaseURL+"/api/v2/upload/upload-chunk", &body)
	if err != nil {
		return fmt.Errorf("create AlfaDisk upload-chunk request: %w", err)
	}
	setAlfaDiskAuthHeaders(req, token, passMD5)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("AlfaDisk upload-chunk request: %w", err)
	}
	var chunk alfaDiskUploadChunkResponse
	if err := decodeAlfaDiskJSONResponse(resp, &chunk); err != nil {
		return err
	}
	if chunk.ErrorUID != "" || chunk.Code != 0 {
		return fmt.Errorf("AlfaDisk upload-chunk failed: %s", chunk.ErrorUID)
	}
	return nil
}

func setAlfaDiskAuthHeaders(req *http.Request, token string, passMD5 string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Vltx-Language", "ru")
	if passMD5 != "" {
		req.Header.Set("Vltx-pass", passMD5)
	}
}

func decodeAlfaDiskJSONResponse(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("AlfaDisk API failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode AlfaDisk API response: %w", err)
	}
	return nil
}

func randomPKCEVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate PKCE verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func parseHTMLFormAction(body string) (string, error) {
	re := regexp.MustCompile(`(?is)<form[^>]+action=["']([^"']+)["']`)
	match := re.FindStringSubmatch(body)
	if len(match) >= 2 {
		return cleanAlfaDiskAuthAction(match[1]), nil
	}

	re = regexp.MustCompile(`(?s)"loginAction"\s*:\s*"((?:\\.|[^"\\])*)"`)
	match = re.FindStringSubmatch(body)
	if len(match) >= 2 {
		return cleanAlfaDiskAuthAction(match[1]), nil
	}

	return "", fmt.Errorf("AlfaDisk auth action not found")
}

func cleanAlfaDiskAuthAction(action string) string {
	action = html.UnescapeString(action)
	action = strings.ReplaceAll(action, `\/`, `/`)
	action = strings.ReplaceAll(action, `\u0026`, `&`)
	return action
}

func codeFromRedirectLocation(location string) (string, error) {
	u, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("parse AlfaDisk auth redirect: %w", err)
	}
	values, err := url.ParseQuery(u.RawFragment)
	if err != nil {
		return "", fmt.Errorf("parse AlfaDisk auth redirect fragment: %w", err)
	}
	code := values.Get("code")
	if code == "" {
		return "", fmt.Errorf("AlfaDisk auth redirect does not include code")
	}
	return code, nil
}

func alfaDiskPasswordMD5(password string) string {
	sum := md5.Sum([]byte(password))
	return fmt.Sprintf("%x", sum)
}
