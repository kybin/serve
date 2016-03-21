package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	method      string
	pathPattern *regexp.Regexp
	serv        func(w http.ResponseWriter, r *http.Request, repo, pth string)
}

func main() {
	http.HandleFunc("/", serveRoot)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveRoot(w http.ResponseWriter, r *http.Request) {
	log.Print(r.URL.Path)

	services := []Service{
		{"GET", regexp.MustCompile("/(.+)/HEAD$"), getHead},
		{"GET", regexp.MustCompile("/(.+)/info/refs$"), getInfoRefs},
		{"GET", regexp.MustCompile("/(.+)/objects/info/alternates$"), getTextFile},
		{"GET", regexp.MustCompile("/(.+)/objects/info/http-alternates$"), getTextFile},
		{"GET", regexp.MustCompile("/(.+)/objects/info/packs$"), getInfoPacks},
		{"GET", regexp.MustCompile("/(.+)/objects/[0-9a-f]{2}/[0-9a-f]{38}$"), getLooseObject},
		{"GET", regexp.MustCompile("/(.+)/objects/pack/pack-[0-9a-f]{40}\\.pack$"), getPackFile},
		{"GET", regexp.MustCompile("/(.+)/objects/pack/pack-[0-9a-f]{40}\\.idx$"), getIdxFile},

		{"POST", regexp.MustCompile("/(.+)/git-upload-pack$"), serviceUpload},
		{"POST", regexp.MustCompile("/(.+)/git-receive-pack$"), serviceReceive},
	}

	for _, s := range services {
		m := s.pathPattern.FindStringSubmatch(r.URL.Path)
		if m == nil {
			continue
		}
		repo := m[1]
		if s.method != r.Method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.serv(w, r, repo, r.URL.Path[1:])
	}
}

func getHead(w http.ResponseWriter, r *http.Request, repo, pth string) {
	headerNoCache(w)
	sendFile(w, r, "text/plain", pth)
}

func getInfoRefs(w http.ResponseWriter, r *http.Request, repo, pth string) {
	r.ParseForm()
	s := r.Form.Get("service")
	if s == "git-upload-pack" || s == "git-receive-pack" {
		// smart protocol
		args := []string{"upload-pack", "--stateless-rpc", "--advertise-refs", repo}
		if s == "git-receive-pack" {
			args = []string{"receive-pack", "--stateless-rpc", "--advertise-refs", repo}
		}
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		headerNoCache(w)
		w.Header().Set("Content-Type", "application/x-"+s+"-advertisement")
		p, err := packetLine("# service=" + s + "\n")
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(p))
		w.Write([]byte("0000")) // flushing
		w.Write(out)
	} else {
		// dumb protocol
		err := exec.Command("git", "update-server-info").Run()
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		headerNoCache(w)
		sendFile(w, r, "text/plain", pth)
	}
}

func getTextFile(w http.ResponseWriter, r *http.Request, repo, pth string) {
	headerNoCache(w)
	sendFile(w, r, "text/plain", pth)
}

func getInfoPacks(w http.ResponseWriter, r *http.Request, repo, pth string) {
	// TODO: pack file validation.
	headerNoCache(w)
	sendFile(w, r, "text/plain; charset=utf-8", pth)
}

func getLooseObject(w http.ResponseWriter, r *http.Request, repo, pth string) {
	headerCacheForever(w)
	sendFile(w, r, "x-git-loose-object", pth)
}

func getPackFile(w http.ResponseWriter, r *http.Request, repo, pth string) {
	headerCacheForever(w)
	sendFile(w, r, "x-git-packed-objects", pth)
}

func getIdxFile(w http.ResponseWriter, r *http.Request, repo, pth string) {
	headerCacheForever(w)
	sendFile(w, r, "x-git-packed-objects-toc", pth)
}

func serviceUpload(w http.ResponseWriter, r *http.Request, repo, pth string) {
	service(w, r, "upload-pack", repo, pth)
}

func serviceReceive(w http.ResponseWriter, r *http.Request, repo, pth string) {
	service(w, r, "receive-pack", repo, pth)
}

func service(w http.ResponseWriter, r *http.Request, s, repo, pth string) {
	w.Header().Set("Content-Type", "application/x-git-"+s+"-result")

	cmd := exec.Command("git", s, "--stateless-rpc", repo)

	in, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	err = cmd.Start()
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Print(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	in.Write(body)
	io.Copy(w, out)
	cmd.Wait()
}

func headerNoCache(w http.ResponseWriter) {
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func headerCacheForever(w http.ResponseWriter) {
	now := time.Now().Unix()
	w.Header().Set("Date", fmt.Sprintf("%v", now))
	w.Header().Set("Expires", fmt.Sprintf("%v", now+31536000))
	w.Header().Set("Cache-Control", "public, max-age=31536000")
}

func sendFile(w http.ResponseWriter, r *http.Request, typ string, pth string) {
	f, err := os.Stat(pth)
	if err != nil {
		if os.IsNotExist(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Length", fmt.Sprintf("%v", f.Size()))
	w.Header().Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
	http.ServeFile(w, r, pth)
}

// packetLine adds 4 digit hex length string to given string.
func packetLine(l string) (string, error) {
	h := strconv.FormatInt(int64(len(l)+4), 16)
	if len(h) > 4 {
		return "", errors.New("packet too long")
	}
	return strings.Repeat("0", 4-len(h)) + h + l, nil
}
