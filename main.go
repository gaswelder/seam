package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func main() {
	keyFile := flag.String("k", "", "keyfile path")
	certFile := flag.String("c", "", "certfile path")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "Hey")
	})
	data, err := ioutil.ReadFile("seam.conf")
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		s := strings.Trim(line, " \n\r\t")
		if s == "" {
			continue
		}
		parts := strings.Split(s, " ")
		if len(parts) != 3 {
			log.Fatal("counld't parse line:", line)
		}
		switch parts[1] {
		case "static":
			http.HandleFunc(parts[0], static(parts[2]))
		case "forward":
			port, err := strconv.Atoi(parts[2])
			if err != nil {
				log.Fatal("faied to parse port", err)
			}
			http.HandleFunc(parts[0], forward(port))
		default:
			log.Fatal("unknown handler:", parts[1])
		}
	}

	if *keyFile != "" {
		log.Println("running HTTPS on port 443")
		log.Fatal(http.ListenAndServeTLS(":443", *certFile, *keyFile, nil))
	} else {
		log.Println("http://localhost:20219")
		log.Fatal(http.ListenAndServe("localhost:20219", nil))
	}
}

func static(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.Split(r.URL.Path, "/")[2:]
		for _, p := range path {
			if p[0] == '.' {
				log.Println("ignoring requested path", r.URL.Path)
				w.WriteHeader(404)
				return
			}
		}
		localPath := dir + r.URL.Path
		log.Printf("@ %s -> %s", r.URL.Path, localPath)
		f, err := os.Open(localPath)
		if err != nil {
			log.Println("file not found", localPath)
			w.WriteHeader(404)
		}
		_, err = io.Copy(w, f)
		if err != nil {
			log.Printf("failed to serve %s: %s", localPath, err.Error())
		}
		defer f.Close()
	}
}

func forward(port int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Figure out the target URL
		parts := strings.Split(r.URL.Path, "/")[2:]
		targetURL := *r.URL
		targetURL.Path = strings.Join(parts, "/")
		targetURL.Scheme = "http"
		targetURL.Host = fmt.Sprintf("localhost:%d", port)
		log.Printf("@ %s %s -> %s", r.Method, r.URL.Path, targetURL.String())

		// Do the target request
		subreq, err := http.NewRequest(r.Method, targetURL.String(), nil)
		if err != nil {
			log.Println(r.RequestURI, "failed:", err)
			http.Error(w, err.Error(), 500)
		}
		resp, err := http.DefaultClient.Do(subreq)
		if err != nil {
			log.Println(r.RequestURI, "failed:", err)
			http.Error(w, err.Error(), 500)
		}

		// Write the results
		w.Header().Add("Content-Type", resp.Header.Get("content-type"))
		w.WriteHeader(resp.StatusCode)
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			log.Println("io.Copy failed", err)
			http.Error(w, err.Error(), 500)
		}
	}
}
