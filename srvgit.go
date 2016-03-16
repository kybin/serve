package main

import (
	"log"
	"net/http"
	"os"
	"regexp"
)

type Service struct {
	method string
	path   string
	serv   func(w http.ResponseWriter, r *http.Request)
}

func main() {
	http.HandleFunc("/", serveRoot)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveRoot(w http.ResponseWriter, r *http.Request) {
	services := []Service{
		{"GET", "/(.+)/HEAD$", getHead},
		{"GET", "/(.+)/info/refs$", getInfoRefs},
		{"GET", "/(.+)/objects/info/alternates$", getTextFile},
		{"GET", "/(.+)/objects/info/http-alternates$", getTextFile},
		{"GET", "/(.+)/objects/info/packs$", getInfoPacks},
		{"GET", "/(.+)/objects/[0-9a-f]{2}/[0-9a-f]{38}$", getLooseObject},
		{"GET", "/(.+)/objects/pack/pack-[0-9a-f]{40}\\.pack$", getPackFile},
		{"GET", "/(.+)/objects/pack/pack-[0-9a-f]{40}\\.idx$", getIdxFile},

		{"POST", "/(.+)/git-upload-pack$", serviceRPC},
		{"POST", "/(.+)/git-receive-pack$", serviceRPC},
	}

	for _, s := range services {
		re := regexp.MustCompile(s.path)
		m := re.FindStringSubmatch(r.URL.Path)
		if m == nil {
			continue
		}

		repo := m[1]
		log.Println(repo)
		stat, err := os.Stat(repo)
		if err != nil {
			// NotFound
			return
		}
		if !stat.IsDir() {
			// NotFound
			return
		}

		if s.method != r.Method {
			// Forbidden?
			return
		}
	}
}

func getHead(w http.ResponseWriter, r *http.Request) {}

func getInfoRefs(w http.ResponseWriter, r *http.Request) {}

func getTextFile(w http.ResponseWriter, r *http.Request) {}

func getInfoPacks(w http.ResponseWriter, r *http.Request) {}

func getLooseObject(w http.ResponseWriter, r *http.Request) {}

func getPackFile(w http.ResponseWriter, r *http.Request) {}

func getIdxFile(w http.ResponseWriter, r *http.Request) {}

func serviceRPC(w http.ResponseWriter, r *http.Request) {}
