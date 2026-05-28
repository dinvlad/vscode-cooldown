package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	vscodeMktplace, _  = url.Parse("https://marketplace.visualstudio.com/_apis/public/gallery")
	openvsxMktplace, _ = url.Parse("https://open-vsx.org")

	allowedOrigins = map[string]bool{
		"vscode-file://vscode-app": true,
	}

	c = &http.Client{}
)

const (
	allowedMethods = "GET, POST, OPTIONS"
)

func main() {
	port := flag.Int("port", 8787, "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET  /vscode/{duration}/{path...}", proxyHandler(vscodeMktplace))
	mux.HandleFunc("POST /vscode/{duration}/{path...}", proxyHandler(vscodeMktplace))
	mux.HandleFunc("GET  /openvsx/{duration}/{path...}", proxyHandler(openvsxMktplace))
	mux.HandleFunc("POST /openvsx/{duration}/{path...}", proxyHandler(openvsxMktplace))

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(*port))
	log.Printf("listening on http://%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, withCORS(mux)))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !allowedOrigins[origin] {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}

		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Methods", allowedMethods)
		h.Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		h.Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func proxyHandler(mktplace *url.URL) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		duration, err := parseDuration(r.PathValue("duration"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		req := r.Clone(r.Context())
		req.URL.Scheme, req.URL.Host = mktplace.Scheme, mktplace.Host
		req.URL.Path = path.Join(mktplace.Path, r.PathValue("path"))
		req.RequestURI, req.Host = "", ""
		req.Header.Del("Accept-Encoding")
		req.Header.Del("User-Agent")

		resp, err := c.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() {
			log.Printf("%-4s %s : %d", req.Method, req.URL.String(), resp.StatusCode)
			if err := resp.Body.Close(); err != nil {
				log.Printf("error closing response body: %v", err)
			}
		}()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if resp.StatusCode >= 400 || r.Method == http.MethodGet {
			writeResponse(w, resp, body)
			return
		}

		// parsing response as nested maps, because we need
		// to preserve all fields for transparent proxying

		var q map[string]any
		if err := json.Unmarshal(body, &q); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		results, ok := q["results"].([]any)
		if !ok {
			http.Error(w, "invalid response: 'results' array not found", http.StatusBadGateway)
			return
		}

		cutoff := time.Now().Add(-duration)
		for _, re := range results {
			res, ok := re.(map[string]any)
			if !ok {
				continue
			}
			extensions, ok := res["extensions"].([]any)
			if !ok {
				continue
			}
			for _, ex := range extensions {
				ext, ok := ex.(map[string]any)
				if !ok {
					continue
				}

				pubName := ""
				publisher, ok := ext["publisher"].(map[string]any)
				if ok {
					pubName, _ = publisher["publisherName"].(string)
				}
				extName, _ := ext["extensionName"].(string)
				versions, ok := ext["versions"].([]any)
				if !ok {
					continue
				}

				vers := []any{}
				for _, ve := range versions {
					v, ok := ve.(map[string]any)
					if !ok {
						continue
					}
					lastUpdated, ok := v["lastUpdated"].(string)
					if !ok {
						continue
					}
					t, err := time.Parse(time.RFC3339, lastUpdated)
					if err != nil {
						log.Println(err)
						continue
					}
					if t.Before(cutoff) {
						vers = append(vers, v)
					} else {
						vs, _ := v["version"].(string)
						log.Printf("excluding %s.%s@%s (published %s)",
							pubName, extName, vs, t.Format("2006-01-02 15:04:05"))
					}
				}
				ext["versions"] = vers
			}
		}

		body, err = json.Marshal(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeResponse(w, resp, body)
	}
}

func writeResponse(w http.ResponseWriter, resp *http.Response, body []byte) {
	maps.Copy(w.Header(), resp.Header)
	w.Header().Del("Content-Encoding")
	w.Header().Del("Content-Length")
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		log.Println(err)
	}
}

func parseDuration(d string) (time.Duration, error) {
	e := errors.New("invalid duration: must be in the format 14d or 4h")
	if hs, ok := strings.CutSuffix(d, "h"); ok {
		hours, err := strconv.Atoi(hs)
		if err != nil {
			return 0, e
		}
		return time.Duration(hours) * time.Hour, nil
	}
	if ds, ok := strings.CutSuffix(d, "d"); ok {
		days, err := strconv.Atoi(ds)
		if err != nil {
			return 0, e
		}
		return time.Duration(days) * time.Hour * 24, nil
	}
	return 0, e
}
