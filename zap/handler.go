package zap

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/unrolled/render"
)

type contextKey string

var (
	appKey contextKey = "app"

	renderer = render.New(render.Options{
		Layout:     "layout",
		Asset:      Asset,
		AssetNames: AssetNames,
		Extensions: []string{".html"},
	})
)

func findAppHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		app, err := findAppForHost(r.Host)
		if err != nil {
			renderer.HTML(w, http.StatusBadGateway, "502", "App Not Found")
			return
		}
		ctx := context.WithValue(r.Context(), appKey, app)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// ZAP HANDLERS

func appHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)

	switch app.Status() {
	case "running":
		app.ServeHTTP(w, r)
	default:
		renderer.HTML(w, http.StatusAccepted, "app", app)
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)
	renderer.HTML(w, http.StatusOK, "app", app)
}

func logHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)
	renderer.HTML(w, http.StatusOK, "log", app)
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)

	if err := app.RestartAdapter(); err != nil {
		log.Println("[app]", app.Config.Host, "internal server error", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
	}

	http.Redirect(w, r, "/zap", http.StatusTemporaryRedirect)
}

func logAPIHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)

	app.WriteLog(w)
}

// NGROK HANDLERS

func ngrokHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)

	renderer.HTML(w, http.StatusOK, "ngrok", app)
}

func startNgrokHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)

	if err := app.StartNgrok(r.Host, 80); err != nil {
		log.Println("[app]", app.Config.Host, "internal server error", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
	}

	http.Redirect(w, r, "/zap/ngrok", http.StatusTemporaryRedirect)
}

// API HANDLERS

func stateAPIHandler(w http.ResponseWriter, r *http.Request) {
	app := r.Context().Value(appKey).(*app)

	content, err := json.MarshalIndent(map[string]interface{}{
		"app":    app,
		"uptime": time.Since(app.Started).String(),
		"status": app.Status(),
	}, "", "  ")
	if err != nil {
		log.Println("[app]", app.Config.Host, "internal server error", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(content)
}

func appsAPIHandler(w http.ResponseWriter, r *http.Request) {
	content, err := json.MarshalIndent(map[string]interface{}{
		"apps": apps,
	}, "", "  ")
	if err != nil {
		log.Println("[app]", "internal server error", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(content)
}
