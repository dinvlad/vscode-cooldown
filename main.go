package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	vscodeMktplace, _  = url.Parse("https://marketplace.visualstudio.com")
	openvsxMktplace, _ = url.Parse("https://open-vsx.org")
	c                  = &http.Client{}
)

func main() {
	port := flag.Int("port", 8787, "port to listen on")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /vscode/{duration}/{path...}", proxyHandler(vscodeMktplace))
	mux.HandleFunc("POST /openvsx/{duration}/{path...}", proxyHandler(openvsxMktplace))

	addr := net.JoinHostPort("localhost", strconv.Itoa(*port))
	fmt.Printf("listening on http://%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
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
		req.URL.Path = "/" + r.PathValue("path")
		req.RequestURI, req.Host = "", ""

		resp, err := c.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() {
			log.Printf("%s : %d", req.URL.String(), resp.StatusCode)
			if err := resp.Body.Close(); err != nil {
				log.Printf("error closing response body: %v", err)
			}
		}()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		if resp.StatusCode >= 400 {
			maps.Copy(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			if _, err := w.Write(body); err != nil {
				log.Printf("%+v", err)
				return
			}
			return
		}

		var q ExtensionQueryResponse
		if err := json.Unmarshal(body, &q); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		cutoff := time.Now().Add(-duration)
		for ri := range q.Results {
			res := &q.Results[ri]
			for ei := range res.Extensions {
				ext := &res.Extensions[ei]
				vers := []Version{}
				for _, v := range ext.Versions {
					if t, err := time.Parse(time.RFC3339, v.LastUpdated); err != nil {
						log.Println(err)
						continue
					} else if t.Before(cutoff) {
						vers = append(vers, v)
					} else {
						log.Printf("filtered: %s.%s@%s (published %s)",
							ext.Publisher.PublisherName, ext.ExtensionName, v.Version, t.Format("2006-01-02 15:04:05"))
					}
				}
				ext.Versions = vers
			}
		}

		body, err = json.Marshal(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		maps.Copy(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if _, err = w.Write(body); err != nil {
			log.Println(err)
		}
	}
}

type ExtensionQueryResponse struct {
	Results []struct {
		Extensions []struct {
			ExtensionName string `json:"extensionName"`
			Publisher     struct {
				PublisherName string `json:"publisherName"`
			} `json:"publisher"`
			Versions []Version `json:"versions"`
		} `json:"extensions"`
	} `json:"results"`
}

type Version struct {
	Version     string `json:"version"`
	LastUpdated string `json:"lastUpdated"`
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
