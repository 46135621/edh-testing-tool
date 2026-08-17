package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"powerlevel/internal/service"
)

var deckIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,64}$`)

type Handler struct {
	analyzer       *service.Analyzer
	logger         *slog.Logger
	requestTimeout time.Duration
}

type analyzeRequest struct {
	URL string `json:"url"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewHandler(analyzer *service.Analyzer, logger *slog.Logger, requestTimeout time.Duration, static http.Handler) http.Handler {
	handler := &Handler{analyzer: analyzer, logger: logger, requestTimeout: requestTimeout}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("POST /api/v1/analyze", handler.analyze)
	mux.Handle("GET /", static)
	return securityHeaders(requestLogger(logger, mux))
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) analyze(w http.ResponseWriter, r *http.Request) {
	body := http.MaxBytesReader(w, r.Body, 16<<10)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request analyzeRequest
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求体必须是只包含 url 字段的 JSON。")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求体只能包含一个 JSON 对象。")
		return
	}

	normalizedURL, deckID, err := ValidateMoxfieldURL(request.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_MOXFIELD_URL", err.Error())
		return
	}
	ctx, cancel := contextWithTimeout(r, h.requestTimeout)
	defer cancel()
	analysis, err := h.analyzer.Analyze(ctx, normalizedURL, deckID)
	if err != nil {
		h.logger.Error("analysis failed", "deck_id", deckID, "error", err)
		writeError(w, http.StatusBadGateway, "ANALYSIS_FAILED", "无法获取或分析该牌组，请确认牌组为公开状态后重试。")
		return
	}
	writeJSON(w, http.StatusOK, analysis)
}

func ValidateMoxfieldURL(raw string) (string, string, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" {
		return "", "", errors.New("请输入以 https:// 开头的 Moxfield 牌组地址。")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "moxfield.com" && host != "www.moxfield.com" {
		return "", "", errors.New("只支持 moxfield.com 的公开牌组地址。")
	}
	if parsed.Port() != "" || parsed.User != nil {
		return "", "", errors.New("Moxfield 地址格式无效。")
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 || parts[0] != "decks" {
		return "", "", errors.New("地址格式应为 https://moxfield.com/decks/{deck-id}。")
	}
	deckID, err := url.PathUnescape(parts[1])
	if err != nil || !deckIDPattern.MatchString(deckID) {
		return "", "", errors.New("Moxfield 牌组 ID 无效。")
	}
	normalized := "https://www.moxfield.com/decks/" + deckID
	return normalized, deckID, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	response := errorResponse{}
	response.Error.Code = code
	response.Error.Message = message
	writeJSON(w, status, response)
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data: https://cards.scryfall.io")
		next.ServeHTTP(w, r)
	})
}
