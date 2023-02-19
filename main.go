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
	port := flag.Int("p", 0, "port")
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
		if len(parts) < 3 {
			log.Fatal("counld't parse line:", line)
		}
		switch parts[1] {
		case "static":
			fmt.Println("static", parts[0], parts[2])
			http.HandleFunc(parts[0], static(parts[2]))
		case "forward":
			port, err := strconv.Atoi(parts[2])
			if err != nil {
				log.Fatal("faied to parse port", err)
			}
			auth := ""
			if len(parts) > 3 {
				auth = parts[3]
			}
			fmt.Println("forward", parts[0], port, auth)
			http.HandleFunc(parts[0], forward(port, auth))
		default:
			log.Fatal("unknown handler:", parts[1])
		}
	}

	if *keyFile != "" {
		if *port == 0 {
			*port = 443
		}
		log.Printf("running HTTPS on port %d", *port)
		log.Fatal(http.ListenAndServeTLS(fmt.Sprintf(":%d", *port), *certFile, *keyFile, nil))
	} else {
		if *port == 0 {
			*port = 20219
		}
		log.Printf("running HTTP on port %d", *port)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
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

func forward(port int, auth string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth != "" {
			username, password, ok := r.BasicAuth()
			if !ok {
				w.Header().Add("WWW-Authenticate", `Basic realm="seam"`)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`Enter username and password`))
				return
			}
			fmt.Println(auth, username, password)
			if fmt.Sprintf("%s:%s", username, password) != auth {
				w.Header().Add("WWW-Authenticate", `Basic realm="seam"`)
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`Incorrect username/password`))
				return
			}
		}

		// Figure out the target URL
		parts := strings.Split(r.URL.Path, "/")[2:]
		targetURL := *r.URL
		targetURL.Path = strings.Join(parts, "/")
		targetURL.Scheme = "http"
		targetURL.Host = fmt.Sprintf("localhost:%d", port)
		log.Printf("forwarding %s %s -> %s", r.Method, r.URL.Path, targetURL.String())

		// Do the target request
		subreq, err := http.NewRequest(r.Method, targetURL.String(), nil)
		if err != nil {
			log.Println(r.RequestURI, "failed:", err)
			http.Error(w, err.Error(), 500)
		}
		for k, v := range r.Header {
			fmt.Println("forwarding", k, "=", v)
			subreq.Header[k] = v
		}
		subreq.Body = r.Body
		resp, err := http.DefaultClient.Do(subreq)
		if err != nil {
			log.Println(r.RequestURI, "failed:", err)
			http.Error(w, err.Error(), 500)
			return
		}

		fmt.Println("got", resp.StatusCode)

		// Write the results
		// w.Header().Add("Content-Type", resp.Header.Get("content-type"))
		for k, v := range resp.Header {
			fmt.Println("forwarding back", k, v)
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			log.Println("io.Copy failed", err)
			http.Error(w, err.Error(), 500)
		}
	}
}
