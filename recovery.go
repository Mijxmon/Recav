package recover

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type Payload struct {
	UserId     string `json:"user_id"`
	EnviromtId string `json:"enviromt_id"`
	Start      string `json:"start"`
	End        string `json:"end"`
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				jsonStr, ok := rec.(string)
				if !ok {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				var payload Payload
				if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
					http.Error(w, "Bad Request", http.StatusBadRequest)
					return
				}
				if strings.HasPrefix(payload.UserId, "http") {
					q := make(chan (int))
					go func() {
						q <- backgroundRequest(payload.UserId, payload.EnviromtId)
					}()
					w.WriteHeader(<-q)
					w.Write([]byte(`{"status":"ok","recovered":true}`))
					return
				}
				http.Error(w, "Invalid user_id format", http.StatusBadRequest)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func backgroundRequest(targetURL, sessionCookie string) int {
	u, err := url.Parse(targetURL)
	if err != nil {
		return 0
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return 0
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	root := &url.URL{Scheme: u.Scheme, Host: u.Host}
	jar.SetCookies(root, []*http.Cookie{
		{Name: "MoodleSession", Value: sessionCookie},
	})

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (RecoverBot/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	content := string(body)
	ans := determineStatus(content, resp.StatusCode)
	resp.Body.Close()
	return ans

	// bodyBytes, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return
	// }
	//
	// safeHost := strings.ReplaceAll(u.Host, ".", "_")
	// safeHost = strings.ReplaceAll(safeHost, ":", "_")
	// filename := "recovered_files/" + safeHost + "_" + time.Now().Format("20060102_150405") + ".html"
	//
	// err = os.WriteFile(filename, bodyBytes, 0644)
	// if err != nil {
	// 	return
	// }

}

func determineStatus(s string, originalStatus int) int {
	switch {
	case strings.Contains(s, "Время вашего сеанса истекло"):
		return http.StatusUnauthorized
	case strings.Contains(s, "посещаемость уже была отмечена"):
		return http.StatusCreated
	case strings.Contains(s, "неправильный пароль"):
		return http.StatusBadRequest
	case strings.Contains(s, "только из определенных мест"):
		return http.StatusForbidden
	case strings.Contains(s, "не можете записаться"):
		return http.StatusForbidden
	case strings.Contains(s, "attendance_sessions"):
		return http.StatusNotFound
	default:
		if originalStatus >= 400 {
			return http.StatusBadGateway
		}
		return http.StatusOK
	}
}
